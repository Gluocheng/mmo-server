package authcfg

import "testing"

func TestPasswordWorkersFromSplitsCPUAndReservesPool(t *testing.T) {
	if got := PasswordWorkersFrom(12, 2, 20, 8); got != 6 {
		t.Fatalf("12 cpu 2 nodes: got %d", got)
	}
	if got := PasswordWorkersFrom(1, 2, 20, 8); got != 2 {
		t.Fatalf("floor: got %d", got)
	}
	if got := PasswordWorkersFrom(64, 1, 20, 8); got != 8 {
		t.Fatalf("pool cap: got %d", got)
	}
}
