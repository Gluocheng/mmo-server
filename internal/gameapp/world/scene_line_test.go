package world

import (
	"testing"

	"github.com/example/mmo-server/internal/code"
	"github.com/example/mmo-server/internal/protocol"
)

func TestArriveMainFillsNextLineAndRejectsWhenFull(t *testing.T) {
	rule := LineRule{SceneID: 81, MaxOnline: 2, MaxLines: 2, SpawnX: 1, SpawnZ: 2}
	ids := []int64{8101, 8102, 8103, 8104, 8105}
	for _, id := range ids {
		defer Leave(id)
	}
	var lines []int32
	for i, id := range ids[:4] {
		view, c := ArriveMain(nil, id, "gate.user", rule)
		if c != code.OK {
			t.Fatalf("arrive %d: %d", i, c)
		}
		lines = append(lines, view.Line)
		if view.X != 1 || view.Z != 2 {
			t.Fatalf("spawn %+v", view)
		}
	}
	if lines[0] != 1 || lines[1] != 1 || lines[2] != 2 || lines[3] != 2 {
		t.Fatalf("lines %v", lines)
	}
	if _, c := ArriveMain(nil, ids[4], "gate.user", rule); c != code.SceneFull {
		t.Fatalf("fifth %d", c)
	}
	if _, ok := PoseOf(ids[4]); ok {
		t.Fatal("rejected player entered")
	}
}

func TestArriveMainNearbySkipsFarAndSelf(t *testing.T) {
	rule := LineRule{SceneID: 82, MaxOnline: 10, MaxLines: 1}
	defer Leave(8201)
	defer Leave(8202)
	defer Leave(8203)
	if _, c := ArriveMain(nil, 8201, "gate.user", rule); c != code.OK {
		t.Fatal(c)
	}
	Place(8202, "gate.user", 82, 1, 100, 0, 0)
	view, c := ArriveMain(nil, 8203, "gate.user", rule)
	if c != code.OK {
		t.Fatal(c)
	}
	if len(view.Nearby) != 1 || view.Nearby[0].UID != 8201 {
		t.Fatalf("nearby %+v", view.Nearby)
	}
}

func TestSwitchKeepsLineAndCooldown(t *testing.T) {
	defer Leave(8301)
	defer Leave(8302)
	Place(8301, "gate.user", 83, 2, 4, 0, 5)
	view, c := SwitchMap(nil, 8301, LineRule{SceneID: 83, MaxOnline: 10, MaxLines: 3, SpawnX: 9}, 10000, 0)
	if c != code.OK || view.Changed || view.Line != 2 || view.X != 4 {
		t.Fatalf("stay %d %+v", c, view)
	}
	view, c = SwitchMap(nil, 8301, LineRule{SceneID: 84, MaxOnline: 10, MaxLines: 1, SpawnX: 7}, 10000, 50)
	if c != code.OK || !view.Changed || view.SceneID != 84 || view.X != 7 {
		t.Fatalf("move %d %+v", c, view)
	}
	if _, c = SwitchMap(nil, 8301, LineRule{SceneID: 83, MaxOnline: 10, MaxLines: 1}, 0, 50); c != code.SceneSwitchCooldown {
		t.Fatalf("cd %d", c)
	}
	if pose, _ := PoseOf(8301); pose.SceneID != 84 {
		t.Fatalf("moved during cd %+v", pose)
	}
	Place(8302, "gate.user", 85, 1, 0, 0, 0)
	if _, c = SwitchMap(nil, 8301, LineRule{SceneID: 85, MaxOnline: 1, MaxLines: 1}, 0, 10050); c != code.SceneFull {
		t.Fatalf("full %d", c)
	}
	if pose, _ := PoseOf(8301); pose.SceneID != 84 {
		t.Fatal("full switch moved")
	}
	view, c = SwitchMap(nil, 8301, LineRule{SceneID: 86, MaxOnline: 5, MaxLines: 1, SpawnX: 3}, 0, 10050)
	if c != code.OK || view.SceneID != 86 {
		t.Fatalf("after failed cd %d %+v", c, view)
	}
}

func TestLeaveFreesLineSlot(t *testing.T) {
	rule := LineRule{SceneID: 87, MaxOnline: 1, MaxLines: 1}
	defer Leave(8701)
	defer Leave(8702)
	if _, c := ArriveMain(nil, 8701, "gate.user", rule); c != code.OK {
		t.Fatal(c)
	}
	if _, c := ArriveMain(nil, 8702, "gate.user", rule); c != code.SceneFull {
		t.Fatalf("before leave %d", c)
	}
	Leave(8701)
	if LineOnline(87, 1) != 0 {
		t.Fatal("slot not freed")
	}
	if _, c := ArriveMain(nil, 8702, "gate.user", rule); c != code.OK {
		t.Fatalf("after leave %d", c)
	}
}

func TestNoticeReachesEveryLine(t *testing.T) {
	defer Leave(8801)
	defer Leave(8802)
	defer Leave(8803)
	Place(8801, "gate.user", 88, 1, 0, 0, 0)
	Place(8802, "gate.user", 88, 5, 0, 0, 0)
	Place(8803, "gate.user", 89, 1, 0, 0, 0)
	if n := BroadcastNotice(nil, 88, &protocol.GmNoticePush{Text: "hi", SceneId: 88}); n != 2 {
		t.Fatalf("notice %d", n)
	}
	got := map[int32]int{}
	for _, row := range ListOnline(88) {
		got[row.Line]++
	}
	if got[1] != 1 || got[5] != 1 || len(got) != 2 {
		t.Fatalf("online %+v", got)
	}
}
