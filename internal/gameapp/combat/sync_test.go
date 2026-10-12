package combat

import (
	"testing"

	"github.com/example/mmo-server/internal/code"
	"github.com/example/mmo-server/internal/gameapp/world"
)

func TestIdleTickSkipsFrameButAdvancesSeq(t *testing.T) {
	seed(t, 64)
	join(t, 97001, 0)
	join(t, 97002, 0)
	if frames := Tick(); len(frames) != 0 {
		t.Fatalf("idle frames=%d", len(frames))
	}
	if frames := Tick(); len(frames) != 0 {
		t.Fatalf("idle frames=%d", len(frames))
	}
	if c := Cast(97001, 1, 97002); c != code.OK {
		t.Fatal(c)
	}
	frames := Tick()
	f, ok := frameOf(frames, 97002)
	if !ok || f.Tick != 3 || len(f.Hits) != 1 || f.Hits[0].TargetHP != 90 {
		t.Fatalf("frame %+v", f)
	}
}

func TestBuffExpirePushesRemovalAndStat(t *testing.T) {
	seedAttr(t)
	join(t, 97101, 0)
	if c := ApplyBuff(97101, 5); c != code.OK {
		t.Fatal(c)
	}
	frames := Tick()
	f, ok := frameOf(frames, 97101)
	if !ok || len(f.Hits) != 0 || len(f.Buffs) != 1 || f.Buffs[0].ID != 5 || f.Buffs[0].Stacks != 1 {
		t.Fatalf("apply frame %+v", f)
	}
	if len(f.Stats) != 1 || f.Stats[0].Attack != 15 || f.Stats[0].MaxHP != 100 {
		t.Fatalf("stat %+v", f.Stats)
	}
	SetClockForTest(5000)
	frames = Tick()
	f, ok = frameOf(frames, 97101)
	if !ok || len(f.Hits) != 0 || f.Tick != 2 {
		t.Fatalf("expire frame %+v", f)
	}
	if len(f.Buffs) != 1 || f.Buffs[0].Stacks != 0 || f.Buffs[0].ID != 5 {
		t.Fatalf("removal %+v", f.Buffs)
	}
	if len(f.Stats) != 1 || f.Stats[0].Attack != 10 || f.Stats[0].HP != 100 {
		t.Fatalf("reverted %+v", f.Stats)
	}
}

func TestTruncationKeepsSelf(t *testing.T) {
	seed(t, 1)
	join(t, 97201, 0)
	join(t, 97202, 0)
	join(t, 97203, 0)
	if c := Cast(97201, 1, 97202); c != code.OK {
		t.Fatal(c)
	}
	if c := Cast(97203, 1, 97201); c != code.OK {
		t.Fatal(c)
	}
	frames := Tick()
	f, ok := frameOf(frames, 97202)
	if !ok || !f.Truncated || len(f.Hits) != 1 || f.Self == nil {
		t.Fatalf("frame %+v", f)
	}
	if f.Self.HP != 90 || f.Self.MaxHP != 100 || len(f.Self.Buffs) != 0 {
		t.Fatalf("self %+v", f.Self)
	}
}

func TestStatChangeDoesNotCrossLine(t *testing.T) {
	seedAttr(t)
	world.Place(98101, "gate.user", world.DefaultSceneID, 1, 0, 0, 0)
	world.Place(98102, "gate.user", world.DefaultSceneID, 2, 0, 0, 0)
	Enter(98101)
	Enter(98102)
	t.Cleanup(func() {
		Leave(98101)
		Leave(98102)
		world.Leave(98101)
		world.Leave(98102)
	})
	if c := ApplyBuff(98102, 5); c != code.OK {
		t.Fatal(c)
	}
	frames := Tick()
	if _, ok := frameOf(frames, 98101); ok {
		t.Fatal("other line saw the buff")
	}
	f, ok := frameOf(frames, 98102)
	if !ok || len(f.Buffs) != 1 || f.Buffs[0].UID != 98102 {
		t.Fatalf("same line %+v", f)
	}
}

func TestVisionEnterAndLeave(t *testing.T) {
	seed(t, 64)
	join(t, 98201, 0)
	join(t, 98202, 100)
	before := world.NearbyUIDs(98201)
	for _, id := range before {
		if id == 98202 {
			t.Fatal("already in aoi")
		}
	}
	if !world.SetPosition(98202, 5, 0, 0) {
		t.Fatal("move in")
	}
	gain, loss := diffIDs(before, world.NearbyUIDs(98201))
	if len(gain) != 1 || gain[0] != 98202 || len(loss) != 0 {
		t.Fatalf("gain=%v loss=%v", gain, loss)
	}
	self, ok := readSnap(98201)
	other, ok2 := readSnap(98202)
	if !ok || !ok2 || self.HP != 100 || other.MaxHP != 100 || other.UID != 98202 {
		t.Fatalf("snaps %+v %+v", self, other)
	}
	before = world.NearbyUIDs(98201)
	if !world.SetPosition(98202, 100, 0, 0) {
		t.Fatal("move out")
	}
	gain, loss = diffIDs(before, world.NearbyUIDs(98201))
	if len(loss) != 1 || loss[0] != 98202 || len(gain) != 0 {
		t.Fatalf("gain=%v loss=%v", gain, loss)
	}
}
