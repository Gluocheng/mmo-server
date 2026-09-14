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
