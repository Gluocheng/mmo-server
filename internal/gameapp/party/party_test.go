package party

import (
	"testing"

	cfacade "github.com/cherry-game/cherry/facade"
	"github.com/example/mmo-server/internal/protocol"
)

type deliverPush struct {
	path  string
	uid   int64
	route string
	v     any
}

func TestDeliverPushesOnlyBoundUIDs(t *testing.T) {
	const (
		boundUID   int64 = 11
		missingUID int64 = 22
		agentPath        = "gate-1.user"
	)
	Bind(boundUID, agentPath)
	t.Cleanup(func() {
		Unbind(boundUID)
		Unbind(missingUID)
	})
	if path, ok := Path(boundUID); !ok || path != agentPath {
		t.Fatalf("Path(%d) = %q, %v", boundUID, path, ok)
	}
	if _, ok := Path(missingUID); ok {
		t.Fatalf("unbound uid %d has a path", missingUID)
	}

	var got []deliverPush
	prev := pushWithUID
	pushWithUID = func(_ cfacade.IActor, path string, uid int64, route string, v any) {
		got = append(got, deliverPush{path: path, uid: uid, route: route, v: v})
	}
	t.Cleanup(func() { pushWithUID = prev })

	(&ActorParties{}).deliver(nil)
	if len(got) != 0 {
		t.Fatalf("nil deliver pushed %+v", got)
	}

	state := &protocol.PartyState{PartyId: 9, LeaderUid: boundUID, Members: []int64{boundUID, missingUID}}
	(&ActorParties{}).deliver(&protocol.PartyDeliver{
		Uids:  []int64{missingUID, boundUID},
		State: state,
	})
	if len(got) != 1 {
		t.Fatalf("state pushes = %d, want 1", len(got))
	}
	if got[0].uid != boundUID || got[0].path != agentPath || got[0].route != "onParty" || got[0].v != state {
		t.Fatalf("state push = %+v", got[0])
	}

	got = nil
	invite := &protocol.PartyInvite{InviteId: 0}
	(&ActorParties{}).deliver(&protocol.PartyDeliver{
		Uids:   []int64{missingUID, boundUID},
		Invite: invite,
	})
	if len(got) != 1 || got[0].uid != boundUID || got[0].path != agentPath || got[0].route != "onPartyInvite" || got[0].v != invite {
		t.Fatalf("cleared invite push = %+v", got)
	}

	got = nil
	(&ActorParties{}).deliver(&protocol.PartyDeliver{
		Uids:   []int64{boundUID, missingUID},
		State:  state,
		Invite: invite,
	})
	if len(got) != 2 || got[0].uid != boundUID || got[0].route != "onParty" || got[1].uid != boundUID || got[1].route != "onPartyInvite" {
		t.Fatalf("both payloads = %+v", got)
	}

	Unbind(boundUID)
	got = nil
	(&ActorParties{}).deliver(&protocol.PartyDeliver{Uids: []int64{boundUID}, State: state})
	if len(got) != 0 {
		t.Fatalf("unbound uid still pushed %+v", got)
	}
}
