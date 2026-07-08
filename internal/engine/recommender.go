package engine

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/sgbudje/runright-platform/internal/types"
)

const (
	cpuHeadroomFactor = 1.20 // 20% headroom above p95 CPU — tolerates brief spikes without thrashing
	memHeadroomFactor = 1.30 // 30% headroom above p95 memory — OOM kills are more disruptive than slowdown
	hoursPerMonth     = 720.0
)

func spotRiskFromInterruptionRate(ratePct float64) string {
	if ratePct <= 0 {
		return ""
	}
	if ratePct < 5 {
		return "low"
	}
	if ratePct <= 15 {
		return "medium"
	}
	return "high"
}

// Recommend returns a ranked list of machine recommendations based on observed
// job metrics and the provided machine catalog.
func Recommend(summary types.MetricsSummary, catalog []types.MachineType) []types.Recommendation {
	detected := summary.DetectedMachine
	poolConstrained := hasPoolConstraints(summary)
	poolCatalog := filterCatalogByPool(catalog, summary)
	if len(poolCatalog) > 0 {
		catalog = poolCatalog
	}

	// Determine the vCPU and memory baselines.
	var detectedVCPUs int
	var currentPricePerHour float64

	if detected != nil {
		detectedVCPUs = detected.VCPUs
		currentPricePerHour = detected.OnDemandPricePerHour
	} else {
		// Fallback: use a sensible default if machine couldn't be detected.
		detectedVCPUs = 2
	}

	// Required resources with headroom.
	// CPU uses p95 usage as a fraction of the detected machine's vCPUs.
	// Memory gets a larger headroom factor because OOM kills are more disruptive than CPU throttling.
	requiredVCPUs := int(math.Ceil(float64(detectedVCPUs) * (summary.CPUPercentP95 / 100.0) * cpuHeadroomFactor))
	if requiredVCPUs < 1 {
		requiredVCPUs = 1
	}
	requiredMemGiB := summary.MemUsedGiBP95 * memHeadroomFactor

	// Ensure minimums make sense.
	if requiredMemGiB < 0.5 {
		requiredMemGiB = 0.5
	}

	currentMonthly := currentPricePerHour * hoursPerMonth

	var results []types.Recommendation

	for _, m := range catalog {
		if m.VCPUs < requiredVCPUs || m.MemoryGiB < requiredMemGiB {
			continue
		}
		tier := classifyTier(m, detected, requiredVCPUs, requiredMemGiB)
		estimatedMonthly := m.OnDemandPricePerHour * hoursPerMonth
		spotMonthly := 0.0
		deltaPercent := 0.0
		spotDeltaPercent := 0.0
		if currentMonthly > 0 {
			deltaPercent = ((estimatedMonthly - currentMonthly) / currentMonthly) * 100
			if m.SpotPricePerHour > 0 {
				spotMonthly = m.SpotPricePerHour * hoursPerMonth
				spotDeltaPercent = ((spotMonthly - currentMonthly) / currentMonthly) * 100
			}
		}
		spotRisk := m.SpotRisk
		if spotRisk == "" {
			spotRisk = spotRiskFromInterruptionRate(m.SpotInterruptionRatePct)
		}
		durationRiskNote := buildDurationRiskNote(m, detected, requiredVCPUs)
		results = append(results, types.Recommendation{
			Machine:             m,
			Tier:                tier,
			EstimatedMonthly:    estimatedMonthly,
			SpotMonthly:         spotMonthly,
			CurrentMonthly:      currentMonthly,
			CostDeltaPercent:    deltaPercent,
			SpotDeltaPercent:    spotDeltaPercent,
			RequiredVCPUs:       requiredVCPUs,
			RequiredMemoryGiB:   requiredMemGiB,
			Reasoning:           buildReasoning(m, summary, requiredVCPUs, requiredMemGiB),
			DurationRiskNote:    durationRiskNote,
			SpotRisk:            spotRisk,
			KubernetesResources: buildK8sResources(summary, requiredVCPUs, requiredMemGiB),
		})
	}

	if len(results) == 0 && poolConstrained && len(catalog) > 0 {
		return fallbackPoolRecommendations(summary, catalog, requiredVCPUs, requiredMemGiB, currentMonthly)
	}

	// Sort: tier first, then prefer same-provider, then by price ascending.
	// This ensures users on GitHub Actions see GitHub machines first within each tier,
	// and users on AWS/GCP see same-provider machines first.
	sort.Slice(results, func(i, j int) bool {
		tierI := tierOrder(results[i].Tier)
		tierJ := tierOrder(results[j].Tier)
		if tierI != tierJ {
			return tierI < tierJ
		}
		
		// Within same tier, prefer same provider as detected machine
		var detectedProvider types.Provider
		if detected != nil {
			detectedProvider = detected.Provider
		}
		sameProviderI := results[i].Machine.Provider == detectedProvider
		sameProviderJ := results[j].Machine.Provider == detectedProvider
		if sameProviderI != sameProviderJ {
			return sameProviderI // true < false, so same-provider machines come first
		}
		
		// Within same tier and provider preference, sort by price ascending
		return results[i].EstimatedMonthly < results[j].EstimatedMonthly
	})

	// Return at most top 10 results per tier to keep output readable.
	results = filterByCostAndPressure(results, summary, detected, requiredVCPUs, requiredMemGiB)
	return cap(results)
}

func filterByCostAndPressure(
	results []types.Recommendation,
	summary types.MetricsSummary,
	detected *types.MachineType,
	requiredVCPUs int,
	requiredMemGiB float64,
) []types.Recommendation {
	if len(results) == 0 {
		return results
	}

	hasCheaper := false
	for _, r := range results {
		if r.CostDeltaPercent < 0 {
			hasCheaper = true
			break
		}
	}

	constrained := isJobConstrained(summary, detected, requiredVCPUs, requiredMemGiB)

	// Only surface positive-delta upgrades when there are no cheaper options
	// and the observed workload indicates pressure on current resources.
	if hasCheaper || !constrained {
		filtered := make([]types.Recommendation, 0, len(results))
		for _, r := range results {
			if r.CostDeltaPercent <= 0 {
				filtered = append(filtered, r)
			}
		}
		if len(filtered) > 0 {
			return filtered
		}
		return results
	}

	// No cheaper option and constrained workload: return only upgrade candidates,
	// ordered cheapest-first among viable expensive recommendations.
	upgrades := make([]types.Recommendation, 0, len(results))
	for _, r := range results {
		if r.CostDeltaPercent > 0 {
			upgrades = append(upgrades, r)
		}
	}
	if len(upgrades) == 0 {
		return results
	}
	sort.SliceStable(upgrades, func(i, j int) bool {
		if upgrades[i].EstimatedMonthly != upgrades[j].EstimatedMonthly {
			return upgrades[i].EstimatedMonthly < upgrades[j].EstimatedMonthly
		}
		return upgrades[i].Machine.ID < upgrades[j].Machine.ID
	})
	return upgrades
}

func isJobConstrained(summary types.MetricsSummary, detected *types.MachineType, requiredVCPUs int, requiredMemGiB float64) bool {
	const (
		cpuPressureThreshold = 85.0
		memPressureThreshold = 90.0
	)

	if summary.CPUPercentP95 >= cpuPressureThreshold {
		return true
	}
	if detected != nil && detected.VCPUs > 0 && requiredVCPUs > detected.VCPUs {
		return true
	}
	if detected != nil && detected.MemoryGiB > 0 {
		if requiredMemGiB > detected.MemoryGiB {
			return true
		}
		if summary.MemUsedGiBP95/detected.MemoryGiB >= memPressureThreshold/100.0 {
			return true
		}
	}
	if summary.MemTotalGiB > 0 && summary.MemUsedGiBP95/summary.MemTotalGiB >= memPressureThreshold/100.0 {
		return true
	}
	return false
}

func hasPoolConstraints(summary types.MetricsSummary) bool {
	return len(summary.AllowedMachineIDs) > 0 || len(summary.AllowedSeries) > 0 || len(summary.AllowedFamilies) > 0
}

func filterCatalogByPool(catalog []types.MachineType, summary types.MetricsSummary) []types.MachineType {
	if !hasPoolConstraints(summary) {
		return catalog
	}
	idSet := make(map[string]struct{}, len(summary.AllowedMachineIDs))
	for _, id := range summary.AllowedMachineIDs {
		norm := strings.TrimSpace(strings.ToLower(id))
		if norm != "" {
			idSet[norm] = struct{}{}
		}
	}
	seriesSet := make(map[string]struct{}, len(summary.AllowedSeries))
	for _, series := range summary.AllowedSeries {
		norm := strings.TrimSpace(strings.ToLower(series))
		if norm != "" {
			seriesSet[norm] = struct{}{}
		}
	}
	familySet := make(map[string]struct{}, len(summary.AllowedFamilies))
	for _, family := range summary.AllowedFamilies {
		norm := strings.TrimSpace(strings.ToLower(family))
		if norm != "" {
			familySet[norm] = struct{}{}
		}
	}

	out := make([]types.MachineType, 0, len(catalog))
	for _, m := range catalog {
		id := strings.ToLower(m.ID)
		series := strings.ToLower(m.Series)
		matched := false
		if len(idSet) > 0 {
			_, matched = idSet[id]
		}
		if !matched && len(seriesSet) > 0 {
			_, matched = seriesSet[series]
		}
		if !matched && len(familySet) > 0 {
			for family := range familySet {
				if strings.HasPrefix(series, family) || strings.HasPrefix(id, family) {
					matched = true
					break
				}
			}
		}
		if matched {
			out = append(out, m)
		}
	}
	return out
}

func fallbackPoolRecommendations(summary types.MetricsSummary, pool []types.MachineType, reqVCPUs int, reqMemGiB float64, currentMonthly float64) []types.Recommendation {
	if len(pool) == 0 {
		return nil
	}
	// When no allowed machine meets requirements, rank by smallest normalized deficit
	// (then lowest price), and return up to 3 best-effort options from the pool.
	sort.Slice(pool, func(i, j int) bool {
		scoreI := deficitScore(pool[i], reqVCPUs, reqMemGiB)
		scoreJ := deficitScore(pool[j], reqVCPUs, reqMemGiB)
		if scoreI != scoreJ {
			return scoreI < scoreJ
		}
		return pool[i].OnDemandPricePerHour < pool[j].OnDemandPricePerHour
	})

	limit := 3
	if len(pool) < limit {
		limit = len(pool)
	}
	out := make([]types.Recommendation, 0, limit)
	for i := 0; i < limit; i++ {
		m := pool[i]
		estimatedMonthly := m.OnDemandPricePerHour * hoursPerMonth
		deltaPercent := 0.0
		if currentMonthly > 0 {
			deltaPercent = ((estimatedMonthly - currentMonthly) / currentMonthly) * 100
		}
		reason := fmt.Sprintf(
			"No machine in the allowed pool satisfies required headroom (%d vCPUs / %.1f GiB). "+
				"Showing closest available pool option: %s (%d vCPUs / %.1f GiB).",
			reqVCPUs, reqMemGiB, m.ID, m.VCPUs, m.MemoryGiB,
		)
		out = append(out, types.Recommendation{
			Machine:           m,
			Tier:              types.TierMoreHeadroom,
			EstimatedMonthly:  estimatedMonthly,
			CurrentMonthly:    currentMonthly,
			CostDeltaPercent:  deltaPercent,
			RequiredVCPUs:     reqVCPUs,
			RequiredMemoryGiB: reqMemGiB,
			Reasoning:         reason,
		})
	}
	return out
}

func deficitScore(m types.MachineType, reqVCPUs int, reqMemGiB float64) float64 {
	vcpuDeficit := 0.0
	if m.VCPUs < reqVCPUs {
		vcpuDeficit = float64(reqVCPUs-m.VCPUs) / float64(reqVCPUs)
	}
	memDeficit := 0.0
	if m.MemoryGiB < reqMemGiB {
		memDeficit = (reqMemGiB - m.MemoryGiB) / reqMemGiB
	}
	return vcpuDeficit + memDeficit
}

func classifyTier(m types.MachineType, detected *types.MachineType, reqVCPUs int, reqMemGiB float64) types.RecommendationTier {
	vcpuRatio := float64(m.VCPUs) / float64(reqVCPUs)
	memRatio := m.MemoryGiB / reqMemGiB

	// Significantly over-provisioned on either dimension — useful if you need burst room.
	// Threshold of 4x CPU or 3x memory to avoid mislabeling the smallest viable machine.
	if vcpuRatio >= 4.0 || memRatio >= 3.0 {
		return types.TierMoreHeadroom
	}
	// Cheaper than the machine currently in use.
	if detected != nil && m.OnDemandPricePerHour < detected.OnDemandPricePerHour {
		return types.TierCheaper
	}
	return types.TierRightSized
}

func buildReasoning(m types.MachineType, s types.MetricsSummary, reqVCPUs int, reqMemGiB float64) string {
	cpuSlack := float64(m.VCPUs-reqVCPUs) / float64(reqVCPUs) * 100
	memSlack := (m.MemoryGiB - reqMemGiB) / reqMemGiB * 100
	return fmt.Sprintf(
		"Job used %.1f%% CPU (p95; avg %.1f%%, peak %.1f%%) and %.2f GiB memory (p95; avg %.2f GiB) across %d samples. "+
			"With %d%% CPU headroom and %d%% memory headroom, requires %d vCPUs and %.1f GiB. "+
			"%s (%d vCPUs, %.1f GiB) gives %.0f%% CPU slack and %.0f%% memory slack.",
		s.CPUPercentP95, s.CPUPercentAvg, s.CPUPercentPeak, s.MemUsedGiBP95, s.MemUsedGiBAvg, s.SampleCount,
		int((cpuHeadroomFactor-1)*100), int((memHeadroomFactor-1)*100),
		reqVCPUs, reqMemGiB,
		m.ID, m.VCPUs, m.MemoryGiB, cpuSlack, memSlack,
	)
}

// buildK8sResources computes Kubernetes resource request/limit values based on
// observed p95 usage. Requests are set to p95 usage (what the pod actually needs
// to be scheduled). Limits are set to peak usage plus a small safety margin so
// a transient spike doesn't trigger an OOM kill or CPU throttle.
func buildK8sResources(s types.MetricsSummary, reqVCPUs int, reqMemGiB float64) *types.KubernetesResources {
	// CPU request: p95 as millicores (1 vCPU = 1000m).
	// We base this on requiredVCPUs which already has the 20% headroom applied.
	cpuRequestMillis := reqVCPUs * 1000

	// CPU limit: peak usage + 20% burst allowance, never below the request.
	cpuPeakMillis := int(math.Ceil(s.CPUPercentPeak / 100.0 * float64(reqVCPUs) * 1000 * 1.20))
	if cpuPeakMillis < cpuRequestMillis {
		cpuPeakMillis = cpuRequestMillis
	}

	// Memory request: p95 with 30% headroom (same as requiredMemGiB).
	memRequestMiB := int(math.Ceil(reqMemGiB * 1024))

	// Memory limit: peak + 20% safety margin, minimum 512 MiB above request.
	memPeakMiB := int(math.Ceil(s.MemUsedGiBPeak * 1024 * 1.20))
	if memPeakMiB < memRequestMiB+512 {
		memPeakMiB = memRequestMiB + 512
	}

	return &types.KubernetesResources{
		CPURequest:    fmt.Sprintf("%dm", cpuRequestMillis),
		CPULimit:      fmt.Sprintf("%dm", cpuPeakMillis),
		MemoryRequest: mibToK8s(memRequestMiB),
		MemoryLimit:   mibToK8s(memPeakMiB),
	}
}

// mibToK8s converts MiB to a Kubernetes memory string (Mi or Gi).
func mibToK8s(mib int) string {
	if mib%1024 == 0 {
		return fmt.Sprintf("%dGi", mib/1024)
	}
	return fmt.Sprintf("%dMi", mib)
}

func tierOrder(t types.RecommendationTier) int {
	switch t {
	case types.TierRightSized:
		return 0
	case types.TierCheaper:
		return 1
	case types.TierMoreHeadroom:
		return 2
	}
	return 99
}

func cap(results []types.Recommendation) []types.Recommendation {
	const maxPerTier = 5
	counts := make(map[types.RecommendationTier]int)
	var out []types.Recommendation
	for _, r := range results {
		if counts[r.Tier] < maxPerTier {
			out = append(out, r)
			counts[r.Tier]++
		}
	}
	return out
}

// buildDurationRiskNote returns a non-empty note when the recommended machine has
// significantly fewer vCPUs than the current one, flagging a possible build slowdown.
func buildDurationRiskNote(m types.MachineType, detected *types.MachineType, reqVCPUs int) string {
	if detected == nil || detected.VCPUs <= 0 {
		return ""
	}
	// Only warn when the candidate machine has <50% of the current vCPU count.
	if float64(m.VCPUs)/float64(detected.VCPUs) >= 0.5 {
		return ""
	}
	return fmt.Sprintf(
		"Recommended %d vCPUs vs current %d vCPUs (%.0f%% reduction). "+
			"CPU-bound steps (compilation, test parallelism) may run slower. "+
			"Validate with a benchmark run before committing to this machine type.",
		m.VCPUs, detected.VCPUs,
		(1-float64(m.VCPUs)/float64(detected.VCPUs))*100,
	)
}
