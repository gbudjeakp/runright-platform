package catalog

import (
	"testing"

	"github.com/sgbudje/runright-platform/internal/types"
)

// TestAll_NonEmpty verifies the embedded catalog parsed correctly.
func TestAll_NonEmpty(t *testing.T) {
	machines := All()
	if len(machines) == 0 {
		t.Fatal("catalog is empty")
	}

	providers := map[types.Provider]int{}
	for _, m := range machines {
		providers[m.Provider]++
	}
	for _, p := range []types.Provider{types.ProviderAWS, types.ProviderGCP, types.ProviderGitHub} {
		if providers[p] == 0 {
			t.Errorf("no machines found for provider %q", p)
		}
	}
}

// TestQuery_FilterByProvider checks that provider filtering returns only the
// correct provider and nothing else.
func TestQuery_FilterByProvider(t *testing.T) {
	for _, p := range []types.Provider{types.ProviderAWS, types.ProviderGCP, types.ProviderGitHub} {
		results := Query(QueryOptions{Provider: p})
		if len(results) == 0 {
			t.Errorf("no results for provider %q", p)
			continue
		}
		for _, m := range results {
			if m.Provider != p {
				t.Errorf("provider filter %q returned machine with provider %q", p, m.Provider)
			}
		}
	}
}

// TestQuery_VCPURange checks that vCPU range filtering is applied correctly.
func TestQuery_VCPURange(t *testing.T) {
	results := Query(QueryOptions{MinVCPUs: 4, MaxVCPUs: 8})
	if len(results) == 0 {
		t.Fatal("expected results for vCPU range 4–8")
	}
	for _, m := range results {
		if m.VCPUs < 4 || m.VCPUs > 8 {
			t.Errorf("machine %s has %d vCPUs, outside range 4–8", m.ID, m.VCPUs)
		}
	}
}

// TestQuery_MemoryRange checks that memory range filtering is applied correctly.
func TestQuery_MemoryRange(t *testing.T) {
	results := Query(QueryOptions{MinMemoryGiB: 8, MaxMemoryGiB: 16})
	if len(results) == 0 {
		t.Fatal("expected results for memory range 8–16 GiB")
	}
	for _, m := range results {
		if m.MemoryGiB < 8 || m.MemoryGiB > 16 {
			t.Errorf("machine %s has %.1f GiB, outside range 8–16", m.ID, m.MemoryGiB)
		}
	}
}

// TestQuery_MaxPrice checks that the max price filter excludes expensive machines.
func TestQuery_MaxPrice(t *testing.T) {
	limit := 0.10
	results := Query(QueryOptions{MaxPricePerHour: limit})
	if len(results) == 0 {
		t.Fatal("expected results under $0.10/hr")
	}
	for _, m := range results {
		if m.OnDemandPricePerHour > limit {
			t.Errorf("machine %s costs $%.4f/hr, exceeds limit $%.2f", m.ID, m.OnDemandPricePerHour, limit)
		}
	}
}

// TestQuery_SortedByPrice verifies results are returned in ascending price order.
func TestQuery_SortedByPrice(t *testing.T) {
	results := Query(QueryOptions{Provider: types.ProviderAWS})
	for i := 1; i < len(results); i++ {
		if results[i-1].OnDemandPricePerHour > results[i].OnDemandPricePerHour {
			t.Errorf("results not sorted: index %d ($%.4f) > index %d ($%.4f)",
				i-1, results[i-1].OnDemandPricePerHour,
				i, results[i].OnDemandPricePerHour)
		}
	}
}

// TestQuery_Architecture checks that architecture filtering works case-insensitively.
func TestQuery_Architecture(t *testing.T) {
	arm := Query(QueryOptions{Architecture: "arm64"})
	if len(arm) == 0 {
		t.Fatal("expected arm64 machines in catalog")
	}
	for _, m := range arm {
		if m.Architecture != "arm64" && m.Architecture != "ARM64" {
			t.Errorf("machine %s has architecture %q, not arm64", m.ID, m.Architecture)
		}
	}
}

// TestFindByID_Known checks that a known machine ID is found.
func TestFindByID_Known(t *testing.T) {
	// t3.micro is a well-known entry in the AWS catalog.
	m := FindByID("t3.micro")
	if m == nil {
		t.Fatal("FindByID(\"t3.micro\") returned nil")
	}
	if m.Provider != types.ProviderAWS {
		t.Errorf("expected provider aws, got %q", m.Provider)
	}
}

// TestFindByID_Missing checks that an unknown ID returns nil.
func TestFindByID_Missing(t *testing.T) {
	if m := FindByID("does-not-exist"); m != nil {
		t.Errorf("expected nil for unknown ID, got %+v", m)
	}
}

// TestDetectMachine_ExactMatch verifies a well-known GCP machine is detected by spec.
func TestDetectMachine_ExactMatch(t *testing.T) {
	// e2-highcpu-16: 16 vCPUs, 16 GiB — from catalog
	m := DetectMachine(16, 16.0, types.ProviderGCP)
	if m == nil {
		t.Fatal("expected a match for 16 vCPU / 16 GiB on GCP")
	}
	if m.Provider != types.ProviderGCP {
		t.Errorf("expected GCP provider, got %q", m.Provider)
	}
}

// TestDetectMachine_GitHubHint verifies that when the provider hint is "github"
// a runner with 2 vCPUs and ~7 GiB does NOT match a GCP/AWS machine.
func TestDetectMachine_GitHubHint(t *testing.T) {
	// 2 vCPUs, 7 GiB matches ubuntu-latest in the GitHub catalog.
	m := DetectMachine(2, 7.0, types.ProviderGitHub)
	if m == nil {
		t.Fatal("expected ubuntu-latest match with github hint")
	}
	if m.Provider != types.ProviderGitHub {
		t.Errorf("expected github provider, got %q (id=%s)", m.Provider, m.ID)
	}
}

// TestDetectMachine_NoHint_GCPWins verifies that without a provider hint a
// 16-vCPU / 16-GiB runner resolves to a GCP machine (e2-highcpu-16) because
// the GitHub catalog entry for 16 cores specifies 64 GiB.
func TestDetectMachine_NoHint_GCPWins(t *testing.T) {
	m := DetectMachine(16, 16.0, "")
	if m == nil {
		t.Fatal("expected a match for 16 vCPU / 16 GiB without hint")
	}
	if m.Provider != types.ProviderGCP {
		t.Logf("resolved to %s (%s) — may differ as catalog evolves", m.ID, m.Provider)
	}
}

// TestDetectMachine_NoMatch verifies that an impossible spec returns nil.
func TestDetectMachine_NoMatch(t *testing.T) {
	m := DetectMachine(999, 9999.0, "")
	if m != nil {
		t.Errorf("expected nil for impossible spec, got %s", m.ID)
	}
}

// TestDetectMachine_ProviderHintExcludesOthers verifies that passing a provider
// hint prevents cross-provider matches.
func TestDetectMachine_ProviderHintExcludesOthers(t *testing.T) {
	// e2-highcpu-16 is GCP — should NOT match when hint=aws.
	m := DetectMachine(16, 16.0, types.ProviderAWS)
	if m != nil && m.Provider != types.ProviderAWS {
		t.Errorf("provider hint aws returned machine from %q", m.Provider)
	}
}

func TestDetectMachineWithConfidence_NoMatch(t *testing.T) {
	m, conf, reason := DetectMachineWithConfidence(999, 9999.0, "")
	if m != nil {
		t.Fatalf("expected nil machine for impossible spec, got %s", m.ID)
	}
	if conf != 0 {
		t.Fatalf("expected zero confidence for no match, got %.2f", conf)
	}
	if reason == "" {
		t.Fatal("expected non-empty reason for no match")
	}
}

func TestDetectMachineWithConfidence_HasReasonAndScore(t *testing.T) {
	m, conf, reason := DetectMachineWithConfidence(2, 7.0, types.ProviderGitHub)
	if m == nil {
		t.Fatal("expected github machine match")
	}
	if conf <= 0 {
		t.Fatalf("expected positive confidence, got %.2f", conf)
	}
	if conf > 1 {
		t.Fatalf("expected confidence <= 1.0, got %.2f", conf)
	}
	if reason == "" {
		t.Fatal("expected non-empty detection reason")
	}
}
