package authcfg

import (
	"testing"

	cprofile "github.com/cherry-game/cherry/profile"
)

func TestPasswordWorkersFromUsesPoolRoom(t *testing.T) {
	if got := PasswordWorkersFrom(40, 8); got != 16 {
		t.Fatalf("40 conns 8 session: got %d", got)
	}
	if got := PasswordWorkersFrom(20, 8); got != 8 {
		t.Fatalf("20 conns 8 session: got %d", got)
	}
	if got := PasswordWorkersFrom(8, 8); got != 2 {
		t.Fatalf("floor: got %d", got)
	}
	// 连接池再大也不超过上限，避免单进程开过多连接。
	if got := PasswordWorkersFrom(200, 8); got != maxPasswordWorkers {
		t.Fatalf("cap: got %d", got)
	}
}

func TestPasswordWorkersFromZeroFallsBack(t *testing.T) {
	if got := PasswordWorkersFrom(0, 0); got != 8 {
		t.Fatalf("fallback 20 conns 8 session: got %d", got)
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
