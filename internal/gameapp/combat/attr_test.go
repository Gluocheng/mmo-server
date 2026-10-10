package combat

import (
	"testing"

	gcruntime "github.com/example/mmo-server/gameconfig/pkg/runtime"
	"github.com/example/mmo-server/internal/code"
	"github.com/example/mmo-server/internal/gameapp/world"
)

func seedAttr(t *testing.T) {
	t.Helper()
	seed(t, 64)
	gcruntime.BuildCombat(
		[]gcruntime.SkillDef{
			{ID: 1, Name: "普攻", Target: "single", CastRange: 3, CooldownMs: 1000, Damage: 0, Factor: 100},
			{ID: 2, Name: "范围斩", Target: "aoe_self", Radius: 5, CooldownMs: 3000, Damage: 0, Factor: 80, BuffID: 1},
			{ID: 4, Name: "禁手", Target: "single", CastRange: 3, CooldownMs: 5000, Damage: 0, Factor: 0, BuffID: 3},
		},
		[]gcruntime.BuffDef{
			{ID: 1, Name: "撕裂", DurationMs: 3000, IntervalMs: 1000, Effect: "dot", Value: 4, MaxStack: 1},
			{ID: 3, Name: "禁手", DurationMs: 2000, Effect: "stun", MaxStack: 1},
			{ID: 5, Name: "战意", DurationMs: 5000, Effect: "attr", Value: 5, MaxStack: 1, Stat: "attack", Mode: "flat"},
			{ID: 6, Name: "怒意", DurationMs: 5000, Effect: "attr", Value: 20, MaxStack: 1, Stat: "attack", Mode: "percent"},
			{ID: 7, Name: "强壮", DurationMs: 5000, Effect: "attr", Value: 50, MaxStack: 1, Stat: "hp", Mode: "flat"},
			{ID: 9, Name: "会心", DurationMs: 5000, Effect: "attr", Value: 10, MaxStack: 1, Stat: "crit_rate", Mode: "percent"},
		},
		gcruntime.CombatConst{MaxHP: 100, TickMs: 100, RespawnMs: 5000, FrameEventCap: 64, Attack: 10, Defense: 0},
	)
	gcruntime.BuildStats([]gcruntime.StatDef{
		{ID: 1, Name: "hp", Desc: "生命", AllowFlat: true, AllowPercent: true, Settle: true},
		{ID: 2, Name: "attack", Desc: "攻击", AllowFlat: true, AllowPercent: true, Settle: true},
		{ID: 3, Name: "defense", Desc: "防御", AllowFlat: true, AllowPercent: true, Settle: true},
		{ID: 4, Name: "crit_rate", Desc: "暴击率", AllowPercent: true},
	})
}

func placeWolf(t *testing.T, uid int64) {
	t.Helper()
	world.PlaceMonster(uid, world.DefaultSceneID, 1, 0, 0, 0, 1)
	mu.Lock()
	units[uid] = &unit{
		hp: 80, maxHP: 80, baseMaxHP: 80,
		defense: 2, baseDefense: 2,
		monster: true,
	}
	mu.Unlock()
	t.Cleanup(func() {
		Leave(uid)
		world.DropMonster(nil, uid)
	})
}

func TestStrikeUsesAttackPercentAndDefense(t *testing.T) {
	seedAttr(t)
	join(t, 91001, 0)
	join(t, 91002, 0)
	placeWolf(t, 91003)
	if c := Cast(91001, 1, 91002); c != code.OK {
		t.Fatal(c)
	}
	Tick()
	if hp, _, _, _ := Snapshot(91002); hp != 90 {
		t.Fatalf("player hp %d", hp)
	}
	SetClockForTest(1000)
	if c := Cast(91001, 1, 91003); c != code.OK {
		t.Fatal(c)
	}
	Tick()
	if hp, _, _, _ := Snapshot(91003); hp != 72 {
		t.Fatalf("wolf basic %d", hp)
	}
	SetClockForTest(2000)
	if c := Cast(91001, 2, 0); c != code.OK {
		t.Fatal(c)
	}
	Tick()
	if hp, _, _, _ := Snapshot(91002); hp != 82 {
		t.Fatalf("aoe player %d", hp)
	}
	if hp, _, _, _ := Snapshot(91003); hp != 66 {
		t.Fatalf("aoe wolf %d", hp)
	}
}

func TestFlatAttackBuffAddsThenExpires(t *testing.T) {
	seedAttr(t)
	join(t, 91101, 0)
	join(t, 91102, 0)
	if c := ApplyBuff(91101, 5); c != code.OK {
		t.Fatal(c)
	}
	if c := Cast(91101, 1, 91102); c != code.OK {
		t.Fatal(c)
	}
	Tick()
	if hp, _, _, _ := Snapshot(91102); hp != 85 {
		t.Fatalf("buffed %d", hp)
	}
	SetClockForTest(5000)
	Tick()
	if c := Cast(91101, 1, 91102); c != code.OK {
		t.Fatal(c)
	}
	Tick()
	if hp, _, _, _ := Snapshot(91102); hp != 75 {
		t.Fatalf("expired %d", hp)
	}
}

func TestAttackPercentAndHpCap(t *testing.T) {
	seedAttr(t)
	join(t, 91201, 0)
	if c := ApplyBuff(91201, 6); c != code.OK {
		t.Fatal(c)
	}
	mu.Lock()
	atk := units[91201].attack
	mu.Unlock()
	if atk != 12 {
		t.Fatalf("attack %d", atk)
	}
	if c := ApplyBuff(91201, 7); c != code.OK {
		t.Fatal(c)
	}
	hp, maxHP, _, ok := Snapshot(91201)
	if !ok || hp != 150 || maxHP != 150 {
		t.Fatalf("raised hp %d max %d", hp, maxHP)
	}
	SetClockForTest(5000)
	Tick()
	hp, maxHP, _, _ = Snapshot(91201)
	if hp != 100 || maxHP != 100 {
		t.Fatalf("clamped hp %d max %d", hp, maxHP)
	}
}

func TestZeroFactorStillAppliesStunAndDotIgnoresAttack(t *testing.T) {
	seedAttr(t)
	join(t, 91301, 0)
	join(t, 91302, 0)
	if c := ApplyBuff(91301, 5); c != code.OK {
		t.Fatal(c)
	}
	if c := Cast(91301, 4, 91302); c != code.OK {
		t.Fatal(c)
	}
	Tick()
	if hp, _, _, _ := Snapshot(91302); hp != 100 {
		t.Fatalf("stun damage %d", hp)
	}
	if c := Cast(91302, 1, 91301); c != code.CombatStunned {
		t.Fatalf("not stunned %d", c)
	}
	if c := ApplyBuff(91302, 1); c != code.CombatStunned {
		t.Fatalf("dot while stunned %d", c)
	}
	Leave(91302)
	world.Leave(91302)
	join(t, 91303, 0)
	if c := ApplyBuff(91301, 1); c != code.OK {
		t.Fatal(c)
	}
	SetClockForTest(1000)
	Tick()
	if hp, _, _, _ := Snapshot(91301); hp != 96 {
		t.Fatalf("dot %d", hp)
	}
}

func TestUnsettledStatBuffRejected(t *testing.T) {
	seedAttr(t)
	join(t, 91401, 0)
	if c := ApplyBuff(91401, 9); c != code.CombatBuffInvalid {
		t.Fatalf("crit buff %d", c)
	}
}
