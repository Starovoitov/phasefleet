package main

import (
	"testing"

	"github.com/Starovoitov/phasefleet/internal/sim"
)

func TestDefaultKuramotoK(t *testing.T) {
	if got := defaultKuramotoK(sim.StrategyKuramoto, 0, false); got != 2 {
		t.Fatalf("omitted K = %v, want 2", got)
	}
	if got := defaultKuramotoK("kuramoto", 0, false); got != 2 {
		t.Fatalf("literal strategy = %v, want 2", got)
	}
	if got := defaultKuramotoK(sim.StrategyKuramoto, 0, true); got != 0 {
		t.Fatalf("explicit 0 = %v, want 0", got)
	}
	if got := defaultKuramotoK(sim.StrategyKuramoto, -1, true); got != -1 {
		t.Fatalf("explicit -1 = %v, want -1", got)
	}
	if got := defaultKuramotoK(sim.StrategyDesync, 0, false); got != 0 {
		t.Fatalf("desync K = %v, want 0", got)
	}
}

func TestMapModeKuramoto(t *testing.T) {
	if mapMode("sync") != sim.StrategyKuramoto || mapMode("kuramoto") != sim.StrategyKuramoto {
		t.Fatalf("sync=%q kuramoto=%q, want %q", mapMode("sync"), mapMode("kuramoto"), sim.StrategyKuramoto)
	}
}
