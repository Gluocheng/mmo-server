package world

import (
	"sync"
	"testing"

	"github.com/example/mmo-server/internal/protocol"
)

func TestAgentPath(t *testing.T) {
	Leave(9)
	if _, ok := AgentPath(9); ok {
		t.Fatal("expected missing")
	}
	Enter(9, "gate-1.user", DefaultSceneID)
	defer Leave(9)
	path, ok := AgentPath(9)
	if !ok || path != "gate-1.user" {
		t.Fatalf("path=%s ok=%v", path, ok)
	}
}

func TestEnterLeave(t *testing.T) {
	Leave(1)
	Leave(2)
	all := Enter(1, "gate-1.user", DefaultSceneID)
	if len(all) != 1 {
		t.Fatalf("enter: got %v", all)
	}
	all = Enter(2, "gate-1.user", DefaultSceneID)
	if len(all) != 2 {
		t.Fatalf("second enter: got %v", all)
	}
	Leave(1)
	all = Enter(3, "gate-1.user", DefaultSceneID)
	if len(all) != 2 {
		t.Fatalf("after leave1: got %v", all)
	}
	Leave(2)
	Leave(3)
}

func TestListOnlineFiltersScene(t *testing.T) {
	Leave(11)
	Leave(12)
	Leave(13)
	Enter(11, "gate-1.user", 1)
	Enter(12, "gate-1.user", 2)
	Enter(13, "gate-1.user", 1)
	defer Leave(11)
	defer Leave(12)
	defer Leave(13)

	all := ListOnline(0)
	if len(all) != 3 {
		t.Fatalf("all=%d %+v", len(all), all)
	}
	s1 := ListOnline(1)
	if len(s1) != 2 {
		t.Fatalf("scene1=%d %+v", len(s1), s1)
	}
	s2 := ListOnline(2)
	if len(s2) != 1 || s2[0].UID != 12 {
		t.Fatalf("scene2=%+v", s2)
	}
}

func TestBroadcastNoticeEmptyRoom(t *testing.T) {
	n := BroadcastNotice(nil, 0, &protocol.GmNoticePush{Text: "hi", SceneId: 0})
	if n != 0 {
		t.Fatalf("expected 0, got %d", n)
	}
	if n := BroadcastNotice(nil, 1, nil); n != 0 {
		t.Fatalf("nil msg expected 0, got %d", n)
	}
}

func TestBroadcastMoveNilMessage(t *testing.T) {
	Leave(102)
	Enter(102, "gate-1.user", DefaultSceneID)
	defer Leave(102)
	BroadcastMove(nil, 102, nil)
}

func TestBroadcastMoveConcurrent(t *testing.T) {
	Leave(101)
	Enter(101, "gate-1.user", DefaultSceneID)
	defer Leave(101)

	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			BroadcastMove(nil, 101, &protocol.MoveBroadcast{Uid: 101, X: float32(n), Y: 0, Z: 0})
		}(i)
	}
	wg.Wait()
}
