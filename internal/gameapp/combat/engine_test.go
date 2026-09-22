package combat

import (
	"testing"

	gcruntime "github.com/example/mmo-server/gameconfig/pkg/runtime"
	"github.com/example/mmo-server/internal/code"
	"github.com/example/mmo-server/internal/gameapp/world"
)

func seed(t *testing.T, cap int32) {
	t.Helper()
	ResetForTest()
	SetClockForTest(0)
	gcruntime.BuildCombat(
		[]gcruntime.SkillDef{
			{ID: 1, Name: "普攻", Target: "single", CastRange: 3, CooldownMs: 1000, Damage: 10},
			{ID: 2, Name: "范围斩", Target: "aoe_self", Radius: 5, CooldownMs: 3000, Damage: 8, BuffID: 1},
			{ID: 3, Name: "斩杀", Target: "single", CastRange: 30, Damage: 100},
		},
		[]gcruntime.BuffDef{
			{ID: 1, Name: "撕裂", DurationMs: 3000, IntervalMs: 1000, Effect: "dot", Value: 4, MaxStack: 1},
		},
		gcruntime.CombatConst{MaxHP: 100, TickMs: 100, RespawnMs: 5000, FrameEventCap: cap},
	)
}

func join(t *testing.T, uid int64, x float32) {
	t.Helper()
	world.Enter(uid, "gate.user", world.DefaultSceneID)
	if !world.SetPosition(uid, x, 0, 0) {
		t.Fatalf("set position %d", uid)
	}
	Enter(uid)
	t.Cleanup(func() {
		Leave(uid)
		world.Leave(uid)
	})
}

func frameOf(frames []Frame, uid int64) (Frame, bool) {
	for _, f := range frames {
		if f.ViewerUID == uid {
			return f, true
		}
	}
	return Frame{}, false
}

func TestAOESkipsSelfFarAndDead(t *testing.T) {
	seed(t, 64)
	const (
		caster = int64(91001)
		near   = int64(91002)
		far    = int64(91003)
		dead   = int64(91004)
	)
	join(t, caster, 0)
	join(t, near, 4)
	join(t, far, 10)
	join(t, dead, 1)
	if c := Cast(caster, 3, dead); c != code.OK {
		t.Fatalf("kill cast %d", c)
	}
	Tick()
	if _, _, alive, ok := Snapshot(dead); !ok || alive {
		t.Fatal("expected dead")
	}
	SetClockForTest(1)
	if c := Cast(caster, 2, 0); c != code.OK {
		t.Fatalf("aoe cast %d", c)
	}
	frames := Tick()
	f, ok := frameOf(frames, caster)
	if !ok {
		t.Fatal("caster frame missing")
	}
	got := map[int64]Hit{}
	for _, h := range f.Hits {
		if h.SkillID == 2 {
			got[h.TargetUID] = h
		}
	}
	if _, ok := got[caster]; ok {
		t.Fatal("aoe hit self")
	}
	if _, ok := got[far]; ok {
		t.Fatal("aoe hit far")
	}
	if _, ok := got[dead]; ok {
		t.Fatal("aoe hit dead")
	}
	h, ok := got[near]
	if !ok || h.Amount != 8 || h.TargetHP != 92 {
		t.Fatalf("near hit %+v", h)
	}
}

func TestBuffTicksThenExpires(t *testing.T) {
	seed(t, 64)
	const uid = int64(92001)
	join(t, uid, 0)
	if c := ApplyBuff(uid, 1); c != code.OK {
		t.Fatalf("apply %d", c)
	}
	Tick()
	if hp, _, _, _ := Snapshot(uid); hp != 100 {
		t.Fatalf("hp at 0 = %d", hp)
	}
	SetClockForTest(1000)
	Tick()
	if hp, _, _, _ := Snapshot(uid); hp != 96 {
		t.Fatalf("hp at 1000 = %d", hp)
	}
	SetClockForTest(2000)
	Tick()
	if hp, _, _, _ := Snapshot(uid); hp != 92 {
		t.Fatalf("hp at 2000 = %d", hp)
	}
	SetClockForTest(3000)
	Tick()
	if hp, _, _, _ := Snapshot(uid); hp != 92 {
		t.Fatalf("hp at 3000 = %d", hp)
	}
}

func TestTickMergesHitsIntoOneFrame(t *testing.T) {
	seed(t, 64)
	join(t, 93001, 0)
	join(t, 93002, 0)
	join(t, 93003, 0)
	if c := Cast(93001, 1, 93002); c != code.OK {
		t.Fatal(c)
	}
	if c := Cast(93003, 1, 93002); c != code.OK {
		t.Fatal(c)
	}
	frames := Tick()
	n := 0
	var viewer Frame
	for _, f := range frames {
		if f.ViewerUID == 93002 {
			n++
			viewer = f
		}
	}
	if n != 1 || len(viewer.Hits) != 2 || viewer.Truncated {
		t.Fatalf("frames for target n=%d hits=%d trunc=%v", n, len(viewer.Hits), viewer.Truncated)
	}
}

func TestFrameCapPrefersSelf(t *testing.T) {
	seed(t, 1)
	join(t, 93101, 0)
	join(t, 93102, 0)
	join(t, 93103, 0)
	if c := Cast(93101, 1, 93102); c != code.OK {
		t.Fatal(c)
	}
	if c := Cast(93103, 1, 93101); c != code.OK {
		t.Fatal(c)
	}
	frames := Tick()
	f, ok := frameOf(frames, 93102)
	if !ok || !f.Truncated || len(f.Hits) != 1 || f.Hits[0].TargetUID != 93102 {
		t.Fatalf("frame %+v", f)
	}
}

func TestFiftyOverlappedUnitsOneFrameEach(t *testing.T) {
	seed(t, 64)
	const n = 50
	base := int64(94000)
	for i := int64(1); i <= n; i++ {
		join(t, base+i, 0)
	}
	if c := Cast(base+1, 2, 0); c != code.OK {
		t.Fatal(c)
	}
	frames := Tick()
	if len(frames) != n {
		t.Fatalf("frames=%d want %d", len(frames), n)
	}
	for _, f := range frames {
		if len(f.Hits) != n-1 {
			t.Fatalf("viewer %d hits=%d", f.ViewerUID, len(f.Hits))
		}
	}
}

func TestCastRejectsRangeCooldownAndDeath(t *testing.T) {
	seed(t, 64)
	join(t, 95001, 0)
	join(t, 95002, 4)
	if c := Cast(95001, 1, 95002); c != code.CombatOutOfRange {
		t.Fatalf("range %d", c)
	}
	if !world.SetPosition(95002, 3, 0, 0) {
		t.Fatal("move")
	}
	if c := Cast(95001, 1, 95002); c != code.OK {
		t.Fatal(c)
	}
	if c := Cast(95001, 1, 95002); c != code.CombatCooldown {
		t.Fatalf("cd %d", c)
	}
	Tick()
	SetClockForTest(10)
	if c := Cast(95001, 3, 95002); c != code.OK {
		t.Fatal(c)
	}
	Tick()
	if c := Cast(95002, 1, 95001); c != code.CombatSelfDead {
		t.Fatalf("dead %d", c)
	}
	SetClockForTest(5010)
	Tick()
	if _, _, alive, ok := Snapshot(95002); !ok || !alive {
		t.Fatal("expected respawn")
	}
}
