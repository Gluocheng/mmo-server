package actor

import (
	"testing"

	"github.com/example/mmo-server/internal/code"
	"github.com/example/mmo-server/internal/gtime"
	"github.com/example/mmo-server/internal/persistence"
	"github.com/example/mmo-server/internal/persistence/model"
	"github.com/example/mmo-server/internal/protocol"
)

func TestRejectIfBanned(t *testing.T) {
	gdb := persistence.UseMemoryDBForTest(t)
	acc := model.Account{UID: 77, Nickname: "locked", Password: "hash"}
	if err := gdb.Create(&acc).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	if c := rejectIfBanned(77); c != code.OK {
		t.Fatalf("expected ok, got %d", c)
	}
	if _, err := persistence.SetAccountBanned(77, true, "gm"); err != nil {
		t.Fatal(err)
	}
	if c := rejectIfBanned(77); c != code.AccountBanned {
		t.Fatalf("expected %d, got %d", code.AccountBanned, c)
	}
}

func TestSetGameTime(t *testing.T) {
	gtime.SetBiasSeconds(0)
	defer gtime.SetBiasSeconds(0)
	s := &ActorSession{}
	if _, c := s.setGameTime(&protocol.GmTimeSetRequest{BiasSeconds: -1}); c != code.GmBadRequest {
		t.Fatalf("neg code=%d", c)
	}
	rsp, c := s.setGameTime(&protocol.GmTimeSetRequest{BiasSeconds: 12})
	if c != code.OK || rsp == nil || rsp.BiasSeconds != 12 {
		t.Fatalf("set: code=%d rsp=%+v", c, rsp)
	}
}
