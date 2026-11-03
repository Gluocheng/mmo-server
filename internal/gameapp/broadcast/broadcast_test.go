package broadcast

import (
	"testing"

	"github.com/example/mmo-server/internal/gameapp/world"
)

func resetQueue(t *testing.T) {
	t.Helper()
	mu.Lock()
	queue = nil
	waiting = map[int64]int{}
	seenOf = map[int64]seen{}
	depth = 0
	capN = QueueCap
	mu.Unlock()
}

func TestMoveCoalescesToOneSlot(t *testing.T) {
	resetQueue(t)
	world.Leave(81001)
	world.Place(81001, "gate.user", 1, 1, 0, 0, 1)
	defer world.Leave(81001)
	NoteSeen(81001)
	if !EnqueueMove(81001) {
		t.Fatal("first enqueue")
	}
	world.SetPosition(81001, 0, 0, 80)
	if !EnqueueMove(81001) {
		t.Fatal("second enqueue")
	}
	mu.Lock()
	n := depth
	mu.Unlock()
	if n != 1 {
		t.Fatalf("depth=%d", n)
	}
	job, ok := pop()
	if !ok || job.uid != 81001 || job.kind != jobMove {
		t.Fatalf("job=%+v ok=%v", job, ok)
	}
	pose, in := world.PoseOf(81001)
	if !in || pose.Z != 80 {
		t.Fatalf("pose=%+v in=%v", pose, in)
	}
}

func TestChatFullAndMoveDrop(t *testing.T) {
	resetQueue(t)
	mu.Lock()
	capN = 2
	mu.Unlock()
	if err := EnqueueChat(1, 1, "a"); err != nil {
		t.Fatal(err)
	}
	if err := EnqueueChat(1, 1, "b"); err != nil {
		t.Fatal(err)
	}
	if err := EnqueueChat(1, 1, "c"); err != ErrFull {
		t.Fatalf("err=%v", err)
	}
	if EnqueueMove(2) {
		t.Fatal("move should drop when the queue is full")
	}
}

func TestNoteSeenDropsStaleMove(t *testing.T) {
	resetQueue(t)
	world.Leave(81002)
	world.Leave(81003)
	world.Place(81002, "gate.user", 1, 1, 0, 0, 0)
	world.Place(81003, "gate.user", 1, 1, 1, 0, 0)
	defer world.Leave(81002)
	defer world.Leave(81003)
	NoteSeen(81002)
	if !EnqueueMove(81002) {
		t.Fatal("enqueue")
	}
	world.SetPosition(81002, 0, 0, 80)
	NoteSeen(81002)
	mu.Lock()
	gen := seenOf[81002].gen
	mu.Unlock()
	flushMove(job{kind: jobMove, uid: 81002, gen: 1})
	mu.Lock()
	got := seenOf[81002].gen
	mu.Unlock()
	if got != gen {
		t.Fatalf("stale flush changed gen %d -> %d", gen, got)
	}
}
