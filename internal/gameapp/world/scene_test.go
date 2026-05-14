package world

import "testing"

func TestEnterLeave(t *testing.T) {
	Leave(1)
	Leave(2)
	all := Enter(1, "gate-1.user")
	if len(all) != 1 {
		t.Fatalf("enter: got %v", all)
	}
	all = Enter(2, "gate-1.user")
	if len(all) != 2 {
		t.Fatalf("second enter: got %v", all)
	}
	Leave(1)
	all = Enter(3, "gate-1.user")
	if len(all) != 2 {
		t.Fatalf("after leave1: got %v", all)
	}
	Leave(2)
	Leave(3)
}
