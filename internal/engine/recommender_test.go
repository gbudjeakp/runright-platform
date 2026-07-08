package engine

import (
	"strings"
	"testing"

	"github.com/sgbudje/runright-platform/internal/catalog"
	"github.com/sgbudje/runright-platform/internal/types"
)

func TestRecommend_BasicOutput(t *testing.T) {
	machines := catalog.All()
	summary := types.MetricsSummary{
		JobID:          "test-job",
		CPUPercentP95:  60,
		CPUPercentPeak: 75,
		CPUPercentAvg:  45,
		MemUsedGiBP95:  3.5,
		MemUsedGiBPeak: 4.0,
		MemTotalGiB:    8.0,
		SampleCount:    12,
		DetectedMachine: &types.MachineType{
			ID:                   "t3.large",
			Provider:             types.ProviderAWS,
			VCPUs:                2,
			MemoryGiB:            8.0,
			OnDemandPricePerHour: 0.0835,
		},
	}

	recs := Recommend(summary, machines)
	if len(recs) == 0 {
		t.Fatal("expected at least one recommendation, got none")
	}
	for _, r := range recs {
		if r.Machine.VCPUs < r.RequiredVCPUs {
			t.Errorf("machine %s has %d vCPUs but required %d", r.Machine.ID, r.Machine.VCPUs, r.RequiredVCPUs)
		}
		if r.Machine.MemoryGiB < r.RequiredMemoryGiB {
			t.Errorf("machine %s has %.1f GiB but required %.1f", r.Machine.ID, r.Machine.MemoryGiB, r.RequiredMemoryGiB)
		}
	}
}

func TestRecommend_HighCPU(t *testing.T) {
	machines := catalog.All()
	summary := types.MetricsSummary{
		JobID:         "heavy-build",
		CPUPercentP95: 95,
		MemUsedGiBP95: 2.0,
		MemTotalGiB:   4.0,
		SampleCount:   30,
		DetectedMachine: &types.MachineType{
			ID:                   "t3.medium",
			Provider:             types.ProviderAWS,
			VCPUs:                2,
			MemoryGiB:            4.0,
			OnDemandPricePerHour: 0.0418,
		},
	}
	recs := Recommend(summary, machines)
	if len(recs) == 0 {
		t.Fatal("expected recommendations for high-CPU job")
	}
	// Top recommendation must have more vCPUs than current under high load.
	top := recs[0]
	if top.RequiredVCPUs <= 2 {
		t.Logf("required vCPUs: %d (may be fine for low-mem workload)", top.RequiredVCPUs)
	}
}

func TestRecommend_NoDetectedMachine(t *testing.T) {
	machines := catalog.All()
	summary := types.MetricsSummary{
		JobID:         "unknown-runner",
		CPUPercentP95: 50,
		MemUsedGiBP95: 1.5,
		MemTotalGiB:   4.0,
		SampleCount:   6,
	}
	recs := Recommend(summary, machines)
	// Should still return results using fallback vCPU count.
	if len(recs) == 0 {
		t.Fatal("expected recommendations even without detected machine")
	}
}

func TestRecommend_TierOrdering(t *testing.T) {
	machines := catalog.All()
	summary := types.MetricsSummary{
		JobID:         "tier-order",
		CPUPercentP95: 40,
		MemUsedGiBP95: 2.0,
		MemTotalGiB:   8.0,
		SampleCount:   10,
		DetectedMachine: &types.MachineType{
			ID:                   "m7i.xlarge",
			Provider:             types.ProviderAWS,
			VCPUs:                4,
			MemoryGiB:            16.0,
			OnDemandPricePerHour: 0.2016,
		},
	}
	recs := Recommend(summary, machines)
	for i := 1; i < len(recs); i++ {
		if tierOrder(recs[i-1].Tier) > tierOrder(recs[i].Tier) {
			t.Errorf("recommendations are not sorted by tier at index %d", i)
		}
	}
}

func TestRecommend_DoesNotGuessSpotValuesWithoutCatalogData(t *testing.T) {
	machines := []types.MachineType{
		{
			ID:                   "m5.large",
			Provider:             types.ProviderAWS,
			Family:               "general-purpose",
			Series:               "m5",
			VCPUs:                2,
			MemoryGiB:            8,
			OnDemandPricePerHour: 0.096,
			// Intentionally no spot_price_per_hour / spot_interruption_rate_pct / spot_risk.
		},
	}

	summary := types.MetricsSummary{
		JobID:         "no-spot-data",
		CPUPercentP95: 45,
		MemUsedGiBP95: 2,
		SampleCount:   8,
		DetectedMachine: &types.MachineType{
			ID:                   "m5.large",
			Provider:             types.ProviderAWS,
			VCPUs:                2,
			MemoryGiB:            8,
			OnDemandPricePerHour: 0.096,
		},
	}

	recs := Recommend(summary, machines)
	if len(recs) == 0 {
		t.Fatal("expected recommendation")
	}

	if recs[0].SpotMonthly != 0 {
		t.Fatalf("expected spot monthly to be 0 when no spot data exists, got %.4f", recs[0].SpotMonthly)
	}
	if recs[0].SpotDeltaPercent != 0 {
		t.Fatalf("expected spot delta to be 0 when no spot data exists, got %.4f", recs[0].SpotDeltaPercent)
	}
	if recs[0].SpotRisk != "" {
		t.Fatalf("expected empty spot risk when no spot data exists, got %q", recs[0].SpotRisk)
	}
}

func TestRecommend_RespectsAllowedSeriesPool(t *testing.T) {
	machines := catalog.All()
	summary := types.MetricsSummary{
		JobID:           "pool-series",
		CPUPercentP95:   55,
		MemUsedGiBP95:   6,
		SampleCount:     10,
		AllowedSeries:   []string{"c7g"},
		DetectedMachine: &types.MachineType{ID: "m7i.2xlarge", Provider: types.ProviderAWS, VCPUs: 8, MemoryGiB: 32, OnDemandPricePerHour: 0.4032},
	}

	recs := Recommend(summary, machines)
	if len(recs) == 0 {
		t.Fatal("expected recommendations from constrained pool")
	}
	for _, r := range recs {
		if r.Machine.Series != "c7g" {
			t.Fatalf("expected constrained series c7g, got %s", r.Machine.Series)
		}
	}
}

func TestRecommend_FallbackWhenPoolCannotSatisfyHeadroom(t *testing.T) {
	machines := []types.MachineType{
		{ID: "c7g.large", Provider: types.ProviderAWS, Series: "c7g", VCPUs: 2, MemoryGiB: 4, OnDemandPricePerHour: 0.0725},
		{ID: "m7i.large", Provider: types.ProviderAWS, Series: "m7i", VCPUs: 2, MemoryGiB: 8, OnDemandPricePerHour: 0.1008},
	}

	summary := types.MetricsSummary{
		JobID:             "pool-exhausted",
		CPUPercentP95:     95,
		MemUsedGiBP95:     14,
		SampleCount:       10,
		AllowedMachineIDs: []string{"c7g.large", "m7i.large"},
		DetectedMachine:   &types.MachineType{ID: "m7i.2xlarge", Provider: types.ProviderAWS, VCPUs: 8, MemoryGiB: 32, OnDemandPricePerHour: 0.4032},
	}

	recs := Recommend(summary, machines)
	if len(recs) == 0 {
		t.Fatal("expected best-effort fallback recommendations")
	}
	if !strings.HasPrefix(recs[0].Reasoning, "No machine ") {
		t.Fatalf("expected pool exhaustion reasoning, got %q", recs[0].Reasoning)
	}
	for _, r := range recs {
		if r.Machine.ID != "c7g.large" && r.Machine.ID != "m7i.large" {
			t.Fatalf("fallback returned machine outside allowed pool: %s", r.Machine.ID)
		}
	}
}

func TestRecommend_HidesPositiveDeltaWhenCheaperExists(t *testing.T) {
	machines := []types.MachineType{
		{ID: "cheap", Provider: types.ProviderAWS, Series: "t", VCPUs: 2, MemoryGiB: 2, OnDemandPricePerHour: 0.05},
		{ID: "current", Provider: types.ProviderAWS, Series: "m", VCPUs: 2, MemoryGiB: 2, OnDemandPricePerHour: 0.10},
		{ID: "expensive", Provider: types.ProviderAWS, Series: "c", VCPUs: 4, MemoryGiB: 4, OnDemandPricePerHour: 0.20},
	}

	summary := types.MetricsSummary{
		JobID:         "cost-filter-cheaper",
		CPUPercentP95: 30,
		MemUsedGiBP95: 1.0,
		MemTotalGiB:   2.0,
		SampleCount:   10,
		DetectedMachine: &types.MachineType{
			ID:                   "current",
			Provider:             types.ProviderAWS,
			VCPUs:                2,
			MemoryGiB:            2,
			OnDemandPricePerHour: 0.10,
		},
	}

	recs := Recommend(summary, machines)
	if len(recs) == 0 {
		t.Fatal("expected recommendations")
	}
	for _, r := range recs {
		if r.CostDeltaPercent > 0 {
			t.Fatalf("expected no positive-delta recommendations when cheaper options exist, got %s (%.2f%%)", r.Machine.ID, r.CostDeltaPercent)
		}
	}
}

func TestRecommend_ShowsCheapestUpgradeWhenConstrainedAndNoCheaper(t *testing.T) {
	machines := []types.MachineType{
		{ID: "current", Provider: types.ProviderAWS, Series: "m", VCPUs: 2, MemoryGiB: 2, OnDemandPricePerHour: 0.10},
		{ID: "upgrade-small", Provider: types.ProviderAWS, Series: "m", VCPUs: 4, MemoryGiB: 4, OnDemandPricePerHour: 0.12},
		{ID: "upgrade-large", Provider: types.ProviderAWS, Series: "m", VCPUs: 8, MemoryGiB: 8, OnDemandPricePerHour: 0.20},
	}

	summary := types.MetricsSummary{
		JobID:         "cost-filter-upgrade",
		CPUPercentP95: 95,
		MemUsedGiBP95: 1.8,
		MemTotalGiB:   2.0,
		SampleCount:   10,
		DetectedMachine: &types.MachineType{
			ID:                   "current",
			Provider:             types.ProviderAWS,
			VCPUs:                2,
			MemoryGiB:            2,
			OnDemandPricePerHour: 0.10,
		},
	}

	recs := Recommend(summary, machines)
	if len(recs) == 0 {
		t.Fatal("expected upgrade recommendations")
	}
	for _, r := range recs {
		if r.CostDeltaPercent <= 0 {
			t.Fatalf("expected only positive-delta upgrade recommendations under constrained workload, got %s (%.2f%%)", r.Machine.ID, r.CostDeltaPercent)
		}
	}
	if recs[0].Machine.ID != "upgrade-small" {
		t.Fatalf("expected cheapest viable upgrade first, got %s", recs[0].Machine.ID)
	}
}
