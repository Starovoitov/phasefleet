package core

import (
	"math"
	"testing"
)

func TestWrap(t *testing.T) {
	twoPi := 2 * math.Pi
	cases := []struct {
		in, want float64
	}{
		{0, 0},
		{twoPi, 0},
		{-twoPi, 0},
		{math.Pi, math.Pi},
		{3 * math.Pi, math.Pi},
		{-math.Pi / 2, 3 * math.Pi / 2},
	}
	for _, tc := range cases {
		got := float64(Wrap(tc.in))
		if math.Abs(got-tc.want) > 1e-12 {
			t.Errorf("Wrap(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestDelta(t *testing.T) {
	cases := []struct {
		a, b Phase
		want float64
	}{
		{0, math.Pi / 2, math.Pi / 2},
		{0, 3 * math.Pi / 2, -math.Pi / 2},
		{math.Pi, 0, math.Pi},
		{0, 0, 0},
	}
	for _, tc := range cases {
		got := Delta(tc.a, tc.b)
		if math.Abs(got-tc.want) > 1e-12 {
			t.Errorf("Delta(%v, %v) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestOrder(t *testing.T) {
	r, psi := Order(nil)
	if r != 0 || psi != 0 {
		t.Fatalf("empty Order = %v, %v", r, psi)
	}

	same := []Phase{math.Pi / 3, math.Pi / 3, math.Pi / 3}
	r, psi = Order(same)
	if math.Abs(r-1) > 1e-12 {
		t.Errorf("locked r = %v, want 1", r)
	}
	if math.Abs(psi-math.Pi/3) > 1e-12 {
		t.Errorf("locked psi = %v, want π/3", psi)
	}

	// Two antipodal clusters cancel. r ≈ 0 is not a uniform splay;
	// DESYNC owns that distinction, Order only reports the first harmonic.
	split := []Phase{0, 0, math.Pi, math.Pi}
	r, _ = Order(split)
	if math.Abs(r) > 1e-12 {
		t.Errorf("antipodal r = %v, want 0", r)
	}
}
