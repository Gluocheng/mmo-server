package gtime

import (
	"testing"
)

func TestSetBiasSeconds(t *testing.T) {
	SetBiasSeconds(0)
	defer SetBiasSeconds(0)

	before := RealNow().Unix()
	SetBiasSeconds(3600)
	after := UnixNow()
	if after < before+3500 {
		t.Fatalf("expected biased now >= before+3500, before=%d after=%d", before, after)
	}
	if !HasBias() {
		t.Fatal("expected HasBias true")
	}
}

func TestSetBiasSecondsNegativeClamped(t *testing.T) {
	SetBiasSeconds(-100)
	if BiasSeconds() != 0 {
		t.Fatalf("expected 0 bias, got %d", BiasSeconds())
	}
	SetBiasSeconds(0)
}
