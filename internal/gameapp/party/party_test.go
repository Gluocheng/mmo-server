package party

import (
	"context"
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

func TestDispatchInviteGoesToOtherNode(t *testing.T) {
	prevOnline := onlineNode
	onlineNode = func(_ context.Context, uid int64) (string, error) {
		switch uid {
		case 2:
			return "10002", nil
		case 4:
			return "10001", nil
		default:
			return "", nil
		}
	}
	t.Cleanup(func() { onlineNode = prevOnline })

	var calls []string
	prevCall := callRemote
	callRemote = func(targetPath, funcName string, _, _ any) int32 {
		calls = append(calls, targetPath+" "+funcName)
		return 0
	}
	t.Cleanup(func() { callRemote = prevCall })

	var pushed []int64
	prevPush := pushWithUID
	pushWithUID = func(_ cfacade.IActor, _ string, uid int64, _ string, _ any) {
		pushed = append(pushed, uid)
	}
	t.Cleanup(func() { pushWithUID = prevPush })

	Dispatch(nil, "10001", &protocol.PartyDeliver{
		Uids:   []int64{2},
		Invite: &protocol.PartyInvite{InviteId: 7, PartyId: 10, LeaderUid: 1},
	})
	if len(pushed) != 0 {
		t.Fatalf("local push %v", pushed)
	}
	if len(calls) != 1 || calls[0] != "10002.party deliver" {
		t.Fatalf("calls %v", calls)
	}

	calls = nil
	pushed = nil
	Dispatch(nil, "10001", &protocol.PartyDeliver{
		Uids:  []int64{3},
		State: &protocol.PartyState{},
	})
	if len(calls) != 0 || len(pushed) != 0 {
		t.Fatalf("offline uid delivered calls=%v pushed=%v", calls, pushed)
	}

	Bind(4, "gate-1.user")
	t.Cleanup(func() { Unbind(4) })
	calls = nil
	pushed = nil
	Dispatch(nil, "10001", &protocol.PartyDeliver{
		Uids:  []int64{4},
		State: &protocol.PartyState{PartyId: 40, LeaderUid: 4, Members: []int64{4}},
	})
	if len(calls) != 0 || len(pushed) != 1 || pushed[0] != 4 {
		t.Fatalf("local deliver calls=%v pushed=%v", calls, pushed)
	}
}
