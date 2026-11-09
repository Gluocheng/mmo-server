package authcfg

import (
	"testing"

	cprofile "github.com/cherry-game/cherry/profile"
)

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

func TestEnabledLoginNodeIDsFromDockerProfile(t *testing.T) {
	if _, err := cprofile.Init("../../configs/mmo-docker.json", "login-1"); err != nil {
		t.Fatal(err)
	}
	got := EnabledLoginNodeIDs()
	if len(got) != 2 || got[0] != "login-1" || got[1] != "login-2" {
		t.Fatalf("got %v", got)
	}
	if n := PasswordWorkers(); n < 2 {
		t.Fatalf("workers %d", n)
	}
}
