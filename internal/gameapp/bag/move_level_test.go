package bag

import "testing"

func TestMoveKeepsEnhanceLevel(t *testing.T) {
	mb := newMemBag()
	mb.set(0, memSlot{itemID: 2001, count: 1, level: 6})
	if err := mb.move(4, 0, 3); err != nil {
		t.Fatal(err)
	}
	got := mb.slots[3]
	if got.itemID != 2001 || got.level != 6 || got.count != 1 {
		t.Fatalf("slot=%+v", got)
	}
	if _, ok := mb.slots[0]; ok {
		t.Fatal("source slot still occupied")
	}
}
