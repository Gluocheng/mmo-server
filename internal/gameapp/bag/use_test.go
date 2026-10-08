package bag

import (
	"testing"

	gcruntime "github.com/example/mmo-server/gameconfig/pkg/runtime"
	"github.com/example/mmo-server/internal/code"
	"github.com/example/mmo-server/internal/gameapp/combat"
	"github.com/example/mmo-server/internal/gameapp/world"
)

func setupPotion(t *testing.T, allowCombat bool) {
	t.Helper()
	combat.ResetForTest()
	combat.SetClockForTest(0)
	gcruntime.BuildCombat(
		[]gcruntime.SkillDef{
			{ID: 1, Name: "普攻", Target: "single", CastRange: 3, CooldownMs: 1000, Damage: 10},
			{ID: 3, Name: "斩杀", Target: "single", CastRange: 30, Damage: 100},
		},
		[]gcruntime.BuffDef{
			{ID: 2, Name: "愈合", DurationMs: 1100, IntervalMs: 1000, Effect: "hot", Value: 30, MaxStack: 1},
		},
		gcruntime.CombatConst{MaxHP: 100, TickMs: 100, RespawnMs: 5000, FrameEventCap: 64},
	)
	gcruntime.BuildScenes([]gcruntime.SceneDef{{
		ID: world.DefaultSceneID, Name: "主城", AllowCombat: allowCombat, MaxOnline: 100, MaxLines: 1,
	}})
}

func enterCombat(t *testing.T, uid int64) {
	t.Helper()
	world.Enter(uid, "gate.user", world.DefaultSceneID)
	if !world.SetPosition(uid, 0, 0, 0) {
		t.Fatalf("set position %d", uid)
	}
	combat.Enter(uid)
	t.Cleanup(func() {
		combat.Leave(uid)
		world.Leave(uid)
	})
}

func TestUseHealsOnNextTickAndConsumesOne(t *testing.T) {
	setupPotion(t, true)
	enterCombat(t, 97001)
	enterCombat(t, 97002)
	if c := combat.Cast(97002, 1, 97001); c != code.OK {
		t.Fatalf("hit %d", c)
	}
	combat.Tick()
	stock := int32(2)
	if c := ApplyUse(97001, 2); c != code.OK {
		t.Fatalf("use %d", c)
	}
	stock = consumeOnSuccess(stock, code.OK)
	combat.SetClockForTest(1000)
	combat.Tick()
	hp, _, alive, ok := combat.Snapshot(97001)
	if !ok || !alive || hp != 100 || stock != 1 {
		t.Fatalf("hp=%d alive=%v ok=%v stock=%d", hp, alive, ok, stock)
	}
}

func TestUseWhileDeadDoesNotConsume(t *testing.T) {
	setupPotion(t, true)
	enterCombat(t, 97101)
	enterCombat(t, 97102)
	if c := combat.Cast(97102, 3, 97101); c != code.OK {
		t.Fatalf("kill %d", c)
	}
	combat.Tick()
	stock := int32(2)
	c := ApplyUse(97101, 2)
	stock = consumeOnSuccess(stock, c)
	if c != code.CombatSelfDead || stock != 2 {
		t.Fatalf("use %d stock %d", c, stock)
	}
}

func TestUseBuffZeroRejected(t *testing.T) {
	setupPotion(t, true)
	enterCombat(t, 97201)
	stock := int32(1)
	c := ApplyUse(97201, 0)
	stock = consumeOnSuccess(stock, c)
	if c != code.ItemNotUsable || stock != 1 {
		t.Fatalf("use %d stock %d", c, stock)
	}
	if hp, _, _, _ := combat.Snapshot(97201); hp != 100 {
		t.Fatalf("hp %d", hp)
	}
}

func TestUseAllowedWhenCombatDisabled(t *testing.T) {
	setupPotion(t, false)
	enterCombat(t, 97301)
	enterCombat(t, 97302)
	if c := combat.Cast(97302, 1, 97301); c != code.SceneCombatDisabled {
		t.Fatalf("cast %d", c)
	}
	stock := int32(1)
	c := ApplyUse(97301, 2)
	stock = consumeOnSuccess(stock, c)
	if c != code.OK || stock != 0 {
		t.Fatalf("use %d stock %d", c, stock)
	}
}

func consumeOnSuccess(stock, result int32) int32 {
	if result != code.OK || stock < 1 {
		return stock
	}
	return stock - 1
}
