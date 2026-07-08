package catalog

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/sgbudje/runright-platform/internal/types"
)

//go:embed data/aws.json
var awsData []byte

//go:embed data/gcp.json
var gcpData []byte

//go:embed data/github.json
var githubData []byte

var allMachines []types.MachineType

func init() {
	var aws []types.MachineType
	if err := json.Unmarshal(awsData, &aws); err != nil {
		panic(fmt.Sprintf("runright: failed to parse aws catalog: %v", err))
	}
	var gcp []types.MachineType
	if err := json.Unmarshal(gcpData, &gcp); err != nil {
		panic(fmt.Sprintf("runright: failed to parse gcp catalog: %v", err))
	}
	var github []types.MachineType
	if err := json.Unmarshal(githubData, &github); err != nil {
		panic(fmt.Sprintf("runright: failed to parse github catalog: %v", err))
	}
	allMachines = append(aws, gcp...)
	allMachines = append(allMachines, github...)
}

// All returns a copy of the full machine catalog.
func All() []types.MachineType {
	out := make([]types.MachineType, len(allMachines))
	copy(out, allMachines)
	return out
}

// QueryOptions allows filtering and sorting the catalog.
type QueryOptions struct {
	Provider        types.Provider
	MinVCPUs        int
	MaxVCPUs        int
	MinMemoryGiB    float64
	MaxMemoryGiB    float64
	MaxPricePerHour float64
	Architecture    string
	Tags            []string
}

// Query filters the catalog and returns matching machines sorted by price.
func Query(opts QueryOptions) []types.MachineType {
	var out []types.MachineType
	for _, m := range allMachines {
		if opts.Provider != "" && m.Provider != opts.Provider {
			continue
		}
		if opts.MinVCPUs > 0 && m.VCPUs < opts.MinVCPUs {
			continue
		}
		if opts.MaxVCPUs > 0 && m.VCPUs > opts.MaxVCPUs {
			continue
		}
		if opts.MinMemoryGiB > 0 && m.MemoryGiB < opts.MinMemoryGiB {
			continue
		}
		if opts.MaxMemoryGiB > 0 && m.MemoryGiB > opts.MaxMemoryGiB {
			continue
		}
		if opts.MaxPricePerHour > 0 && m.OnDemandPricePerHour > opts.MaxPricePerHour {
			continue
		}
		if opts.Architecture != "" && !strings.EqualFold(m.Architecture, opts.Architecture) {
			continue
		}
		if len(opts.Tags) > 0 && !hasTags(m, opts.Tags) {
			continue
		}
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].OnDemandPricePerHour < out[j].OnDemandPricePerHour
	})
	return out
}

// FindByID returns a machine type by its ID, or nil if not found.
func FindByID(id string) *types.MachineType {
	for i := range allMachines {
		if allMachines[i].ID == id {
			return &allMachines[i]
		}
	}
	return nil
}

// DetectMachine matches vCPU count and total memory against the catalog to
// identify what machine the current runner is likely running on.
// When providerHint is non-empty only machines from that provider are considered,
// which avoids false positives when the CI platform is known (e.g. GitHub Actions
// runners are hosted on Azure/GitHub infrastructure, not AWS or GCP).
func DetectMachine(vcpus int, memTotalGiB float64, providerHint types.Provider) *types.MachineType {
	m, _, _ := DetectMachineWithConfidence(vcpus, memTotalGiB, providerHint)
	return m
}

// DetectMachineWithConfidence resolves a likely machine and also returns a
// confidence score (0..1) and a short reason string.
func DetectMachineWithConfidence(vcpus int, memTotalGiB float64, providerHint types.Provider) (*types.MachineType, float64, string) {
	const memToleranceGiB = 2.0
	type candidate struct {
		m    *types.MachineType
		diff float64
	}
	candidates := make([]candidate, 0, 8)
	for i := range allMachines {
		m := &allMachines[i]
		if providerHint != "" && m.Provider != providerHint {
			continue
		}
		if m.VCPUs != vcpus {
			continue
		}
		diff := m.MemoryGiB - memTotalGiB
		if diff < 0 {
			diff = -diff
		}
		if diff <= memToleranceGiB {
			candidates = append(candidates, candidate{m: m, diff: diff})
		}
	}
	if len(candidates) == 0 {
		return nil, 0, "no matching catalog entry within memory tolerance"
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].diff == candidates[j].diff {
			return candidates[i].m.OnDemandPricePerHour < candidates[j].m.OnDemandPricePerHour
		}
		return candidates[i].diff < candidates[j].diff
	})

	best := candidates[0]
	confidence := confidenceFromMemDiff(best.diff)

	if providerHint == "" {
		providers := map[types.Provider]struct{}{}
		for _, c := range candidates {
			providers[c.m.Provider] = struct{}{}
		}
		if len(providers) > 1 {
			confidence -= 0.20
		}
	}

	if len(candidates) > 1 {
		second := candidates[1]
		if math.Abs(second.diff-best.diff) <= 0.10 {
			confidence -= 0.15
		}
	}

	if confidence < 0.05 {
		confidence = 0.05
	}
	if confidence > 0.99 {
		confidence = 0.99
	}

	reason := fmt.Sprintf("matched by vCPU=%d and memory diff %.2f GiB", vcpus, best.diff)
	return best.m, confidence, reason
}

func confidenceFromMemDiff(diff float64) float64 {
	switch {
	case diff <= 0.25:
		return 0.98
	case diff <= 0.50:
		return 0.90
	case diff <= 1.00:
		return 0.75
	default:
		return 0.60
	}
}

func hasTags(m types.MachineType, required []string) bool {
	tagSet := make(map[string]struct{}, len(m.Tags))
	for _, t := range m.Tags {
		tagSet[t] = struct{}{}
	}
	for _, r := range required {
		if _, ok := tagSet[r]; !ok {
			return false
		}
	}
	return true
}
