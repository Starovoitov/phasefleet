package clienv

import (
	"os"
	"testing"
)

func TestStringBlankIsFallback(t *testing.T) {
	t.Setenv("PHASEFLEET_STRATEGY", "")
	if got := String("PHASEFLEET_STRATEGY", "kuramoto"); got != "kuramoto" {
		t.Fatalf("blank = %q", got)
	}
	t.Setenv("PHASEFLEET_STRATEGY", "desync")
	if got := String("PHASEFLEET_STRATEGY", "kuramoto"); got != "desync" {
		t.Fatalf("set = %q", got)
	}
}

func TestOptionalKeepsBlank(t *testing.T) {
	t.Setenv("PHASEFLEET_METRICS", "")
	if got := Optional("PHASEFLEET_METRICS", ":9100"); got != "" {
		t.Fatalf("blank = %q, want empty", got)
	}
	osUnset(t, "PHASEFLEET_METRICS_UNSET")
	if got := Optional("PHASEFLEET_METRICS_UNSET", ":9100"); got != ":9100" {
		t.Fatalf("unset = %q", got)
	}
}

func TestFloat(t *testing.T) {
	t.Setenv("PHASEFLEET_K", "0")
	v, ok, err := Float("PHASEFLEET_K")
	if err != nil || !ok || v != 0 {
		t.Fatalf("zero = %v ok=%v err=%v", v, ok, err)
	}
	t.Setenv("PHASEFLEET_K", "")
	if _, ok, err = Float("PHASEFLEET_K"); err != nil || ok {
		t.Fatalf("blank ok=%v err=%v, want absent", ok, err)
	}
	t.Setenv("PHASEFLEET_K", "nope")
	if _, _, err = Float("PHASEFLEET_K"); err == nil {
		t.Fatal("expected a parse error")
	}
}

func osUnset(t *testing.T, key string) {
	t.Helper()
	t.Setenv(key, "x")
	if err := os.Unsetenv(key); err != nil {
		t.Fatal(err)
	}
}
