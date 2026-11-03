package admit

import "testing"

func TestAcquireSpillsThenBusy(t *testing.T) {
	g := New(2)
	for i := 0; i < int(QueueLimit); i++ {
		id, ok := g.AcquireFrom(0)
		if !ok || id != 0 {
			t.Fatalf("fill 0: i=%d id=%d ok=%v", i, id, ok)
		}
	}
	id, ok := g.AcquireFrom(0)
	if !ok || id != 1 {
		t.Fatalf("spill: id=%d ok=%v", id, ok)
	}
	for i := 0; i < int(QueueLimit)-1; i++ {
		if _, ok := g.AcquireFrom(1); !ok {
			t.Fatalf("fill 1 at %d", i)
		}
	}
	if _, ok := g.AcquireFrom(0); ok {
		t.Fatal("expected busy")
	}
	g.Release(0)
	id, ok = g.AcquireFrom(0)
	if !ok || id != 0 {
		t.Fatalf("after release: id=%d ok=%v", id, ok)
	}
	g.Release(0)
	g.Release(0)
	if g.Pending(0) < 0 {
		t.Fatal("pending below zero")
	}
}
