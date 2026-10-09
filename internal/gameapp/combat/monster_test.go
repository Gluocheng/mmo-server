package combat

import (
	"math"
	"sync"
	"testing"

	cherrySnowflake "github.com/cherry-game/cherry/extend/snowflake"
	gcruntime "github.com/example/mmo-server/gameconfig/pkg/runtime"
	"github.com/example/mmo-server/internal/code"
	"github.com/example/mmo-server/internal/gameapp/world"
)

var snowflakeOnce sync.Once

func ensureSnowflake() {
	snowflakeOnce.Do(func() {
		cherrySnowflake.SetDefaultNode(10001)
	})
}

func wolfDef() gcruntime.MonsterDef {
	return gcruntime.MonsterDef{
		ID: 1, Name: "野狼", Kind: "mob", HP: 80, Attack: 6, Defense: 2,
		MoveSpeed: 4, AttackRange: 2, AttackIntervalMs: 1200,
		AggroRange: 6, LeashRange: 12,
	}
}

func wildSpawn() gcruntime.SpawnDef {
	return gcruntime.SpawnDef{
		ID: 1, Feature: "wild", SceneID: 3, Line: 1, MonsterID: 1,
		X: 18, Y: 0, Z: 10, Count: 1, RespawnMs: 8000,
	}
}

func setupWolf(t *testing.T) int64 {
	t.Helper()
	ensureSnowflake()
	ResetForTest()
	SetClockForTest(0)
	gcruntime.BuildCombat(
		[]gcruntime.SkillDef{
			{ID: 1, Name: "普攻", Target: "single", CastRange: 30, Damage: 10},
			{ID: 3, Name: "斩杀", Target: "single", CastRange: 30, Damage: 100},
			{ID: 5, Name: "空挥", Target: "single", CastRange: 30, Damage: 0},
		},
		[]gcruntime.BuffDef{
			{ID: 3, Name: "禁手", DurationMs: 2000, Effect: "stun", MaxStack: 1},
		},
		gcruntime.CombatConst{MaxHP: 100, TickMs: 100, RespawnMs: 5000, FrameEventCap: 64},
	)
	gcruntime.BuildScenes([]gcruntime.SceneDef{
		{ID: 1, Name: "主城", Main: true, MaxOnline: 200, MaxLines: 5},
		{ID: 3, Name: "荒野", AllowCombat: true, MaxOnline: 50, MaxLines: 1, SpawnX: 10, SpawnZ: 10},
	})
	gcruntime.BuildMonsters([]gcruntime.MonsterDef{wolfDef()}, []gcruntime.SpawnDef{wildSpawn()})
	SyncSpawns(nil)
	uid, ok := MonsterBySlot(1, 1, 0)
	if !ok {
		t.Fatal("wolf missing")
	}
	t.Cleanup(func() {
		Leave(uid)
		world.Leave(uid)
	})
	return uid
}

func joinScene(t *testing.T, uid int64, scene int32, x, z float32) {
	t.Helper()
	world.Enter(uid, "gate.user", scene)
	if !world.SetPosition(uid, x, 0, z) {
		t.Fatalf("move %d", uid)
	}
	Enter(uid)
	t.Cleanup(func() {
		Leave(uid)
		world.Leave(uid)
	})
}

func near(t *testing.T, got, want float32) {
	t.Helper()
	if math.Abs(float64(got-want)) > 0.02 {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestWolfVisibleAndNotCounted(t *testing.T) {
	wolf := setupWolf(t)
	if world.LineOnline(3, 1) != 0 {
		t.Fatal("wolf counted")
	}
	view, c := world.ArriveMain(nil, 97001, "gate.user", world.LineRule{
		SceneID: 3, MaxOnline: 50, MaxLines: 1, SpawnX: 10, SpawnZ: 10,
	})
	if c != code.OK {
		t.Fatal(c)
	}
	t.Cleanup(func() { world.Leave(97001) })
	if world.LineOnline(3, 1) != 1 {
		t.Fatalf("online %d", world.LineOnline(3, 1))
	}
	if len(view.Nearby) != 1 || view.Nearby[0].UID != wolf || view.Nearby[0].ActorType != world.ActorMonster || view.Nearby[0].ConfigID != 1 {
		t.Fatalf("nearby %+v", view.Nearby)
	}
}

func TestHitWolfSubtractsDefenseAndZeroStaysZero(t *testing.T) {
	wolf := setupWolf(t)
	joinScene(t, 97101, 3, 18, 10)
	if c := Cast(97101, 5, wolf); c != code.OK {
		t.Fatal(c)
	}
	Tick()
	if hp, _, _, _ := Snapshot(wolf); hp != 80 {
		t.Fatalf("zero damage hp %d", hp)
	}
	if c := Cast(97101, 1, wolf); c != code.OK {
		t.Fatal(c)
	}
	Tick()
	if hp, _, _, _ := Snapshot(wolf); hp != 72 {
		t.Fatalf("hp %d", hp)
	}
}

func TestWolfChasesThenAttacksOnInterval(t *testing.T) {
	wolf := setupWolf(t)
	joinScene(t, 97201, 3, 10, 10)
	Tick()
	pose, ok := world.PoseOf(wolf)
	if !ok || pose.X != 18 {
		t.Fatalf("should stay %+v", pose)
	}
	if !world.SetPosition(97201, 13, 0, 10) {
		t.Fatal("move player")
	}
	Tick()
	pose, _ = world.PoseOf(wolf)
	near(t, pose.X, 17.6)
	if hp, _, _, _ := Snapshot(97201); hp != 100 {
		t.Fatalf("chasing should not hit %d", hp)
	}
}

func TestWolfAttacksOnIntervalOnceInRange(t *testing.T) {
	wolf := setupWolf(t)
	joinScene(t, 97202, 3, 16, 10)
	Tick()
	if hp, _, _, _ := Snapshot(97202); hp != 94 {
		t.Fatalf("first hit %d", hp)
	}
	pose, _ := world.PoseOf(wolf)
	if pose.X != 18 {
		t.Fatalf("should stop %v", pose.X)
	}
	SetClockForTest(1199)
	Tick()
	if hp, _, _, _ := Snapshot(97202); hp != 94 {
		t.Fatalf("interval %d", hp)
	}
	SetClockForTest(1200)
	Tick()
	if hp, _, _, _ := Snapshot(97202); hp != 88 {
		t.Fatalf("second hit %d", hp)
	}
}

func TestWolfReturnsHomeWhenLeftOrLeashed(t *testing.T) {
	wolf := setupWolf(t)
	joinScene(t, 97301, 3, 13, 10)
	Tick()
	pose, _ := world.PoseOf(wolf)
	chasing := pose.X
	if !world.SetPosition(97301, 0, 0, 10) {
		t.Fatal("leave")
	}
	for i := 0; i < 40; i++ {
		Tick()
		pose, _ = world.PoseOf(wolf)
		if pose.X == 18 && pose.Z == 10 {
			break
		}
	}
	pose, _ = world.PoseOf(wolf)
	if pose.X != 18 || pose.Z != 10 {
		t.Fatalf("not home %+v from %v", pose, chasing)
	}
	if hp, _, _, _ := Snapshot(97301); hp != 100 {
		t.Fatalf("return attacked %d", hp)
	}

	if !world.SetPosition(wolf, 31, 0, 10) || !world.SetPosition(97301, 31, 0, 10) {
		t.Fatal("leash place")
	}
	Tick()
	pose, _ = world.PoseOf(wolf)
	near(t, pose.X, 30.6)
	if hp, _, _, _ := Snapshot(97301); hp != 100 {
		t.Fatalf("leash attacked %d", hp)
	}
	if !world.SetPosition(97301, 0, 0, 10) {
		t.Fatal("clear leash target")
	}
	for i := 0; i < 40; i++ {
		Tick()
		pose, _ = world.PoseOf(wolf)
		if hp, _, _, _ := Snapshot(97301); hp != 100 {
			t.Fatalf("leash attacked %d", hp)
		}
		if pose.X == 18 && pose.Z == 10 {
			return
		}
	}
	t.Fatalf("leash did not reach home %+v", pose)
}

func TestStunStopsWolfButPlayerCanMove(t *testing.T) {
	wolf := setupWolf(t)
	joinScene(t, 97401, 3, 16, 10)
	if c := ApplyBuff(wolf, 3); c != code.OK {
		t.Fatal(c)
	}
	Tick()
	pose, _ := world.PoseOf(wolf)
	if pose.X != 18 {
		t.Fatalf("stunned moved %v", pose.X)
	}
	if hp, _, _, _ := Snapshot(97401); hp != 100 {
		t.Fatalf("stunned attacked %d", hp)
	}
	if c := ApplyBuff(97401, 3); c != code.OK {
		t.Fatal(c)
	}
	if !world.SetPosition(97401, 12, 0, 10) {
		t.Fatal("player move")
	}
	if c := Cast(97401, 1, wolf); c != code.CombatStunned {
		t.Fatalf("cast %d", c)
	}
}

func TestWolfRespawnsAtHomeAndPlayerRespawnsInPlace(t *testing.T) {
	wolf := setupWolf(t)
	joinScene(t, 97501, 3, 10, 10)
	joinScene(t, 97502, 3, 10, 10)
	if !world.SetPosition(wolf, 30, 0, 10) {
		t.Fatal("drag")
	}
	if c := ApplyBuff(wolf, 3); c != code.OK {
		t.Fatal(c)
	}
	for i := 0; i < 10; i++ {
		if c := Cast(97501, 1, wolf); c != code.OK {
			t.Fatalf("hit %d code %d", i, c)
		}
		Tick()
	}
	if _, _, alive, _ := Snapshot(wolf); alive {
		t.Fatal("expected dead")
	}
	if c := Cast(97501, 1, wolf); c != code.CombatTargetDead {
		t.Fatalf("corpse %d", c)
	}
	pose, _ := world.PoseOf(wolf)
	if pose.X != 30 {
		t.Fatalf("corpse moved %v", pose.X)
	}
	SetClockForTest(7999)
	Tick()
	if _, _, alive, _ := Snapshot(wolf); alive {
		t.Fatal("early respawn")
	}
	SetClockForTest(8000)
	Tick()
	hp, _, alive, ok := Snapshot(wolf)
	pose, _ = world.PoseOf(wolf)
	if !ok || !alive || hp != 80 || pose.X != 18 || pose.Z != 10 {
		t.Fatalf("respawn hp %d alive %v pose %+v", hp, alive, pose)
	}
	again, ok := MonsterBySlot(1, 1, 0)
	if !ok || again != wolf {
		t.Fatalf("uid %d -> %d", wolf, again)
	}

	if c := Cast(97501, 3, 97502); c != code.OK {
		t.Fatal(c)
	}
	Tick()
	SetClockForTest(8000 + 5000)
	Tick()
	if _, _, alive, ok := Snapshot(97502); !ok || !alive {
		t.Fatal("player should respawn at 5s")
	}
	playerPose, _ := world.PoseOf(97502)
	if playerPose.X != 10 {
		t.Fatalf("player moved on respawn %v", playerPose.X)
	}
}

func TestReloadKeepsWolfUntilSpawnChanges(t *testing.T) {
	wolf := setupWolf(t)
	joinScene(t, 97601, 3, 18, 10)
	if c := Cast(97601, 1, wolf); c != code.OK {
		t.Fatal(c)
	}
	Tick()
	SyncSpawns(nil)
	kept, ok := MonsterBySlot(1, 1, 0)
	if !ok || kept != wolf {
		t.Fatal("uid changed")
	}
	if hp, _, _, _ := Snapshot(wolf); hp != 72 {
		t.Fatalf("hp reset %d", hp)
	}
	spawn := wildSpawn()
	spawn.X = 20
	gcruntime.BuildMonsters([]gcruntime.MonsterDef{wolfDef()}, []gcruntime.SpawnDef{spawn})
	SyncSpawns(nil)
	if _, _, _, ok := Snapshot(wolf); ok {
		t.Fatal("old wolf still live")
	}
	next, ok := MonsterBySlot(1, 1, 0)
	if !ok || next == wolf {
		t.Fatal("new wolf missing")
	}
	pose, ok := world.PoseOf(next)
	hp, _, alive, snapOK := Snapshot(next)
	if !ok || !snapOK || !alive || hp != 80 || pose.X != 20 {
		t.Fatalf("new hp %d pose %+v", hp, pose)
	}
	t.Cleanup(func() {
		Leave(next)
		world.Leave(next)
	})
}

func TestKillDamageCapsAtRemainingHP(t *testing.T) {
	ResetForTest()
	mu.Lock()
	units[9] = &unit{hp: 5, maxHP: 5, monster: true, templateID: 2, sceneID: 4, line: 1}
	units[1] = &unit{hp: 100, maxHP: 100, playerID: 70}
	applyDamage(units[9], 9, 1, 8, 1000)
	mu.Unlock()
	kills := TakeKills()
	if len(kills) != 1 || kills[0].LastHit != 1 || len(kills[0].Hits) != 1 || kills[0].Hits[0].Damage != 5 || kills[0].Hits[0].PlayerID != 70 {
		t.Fatalf("%+v", kills)
	}
	mu.Lock()
	units[9].dead = false
	units[9].hp = 5
	applyDamage(units[9], 9, 1, 1, 2000)
	applyDamage(units[9], 9, 1, 9, 2001)
	mu.Unlock()
	kills = TakeKills()
	if len(kills) != 1 || kills[0].Hits[0].Damage != 5 {
		t.Fatalf("respawn ledger %+v", kills)
	}
}

func TestWorldBossLineZeroSpawnsEachLine(t *testing.T) {
	ensureSnowflake()
	ResetForTest()
	gcruntime.BuildScenes([]gcruntime.SceneDef{
		{ID: 3, Name: "荒野", AllowCombat: true, MaxOnline: 50, MaxLines: 1},
		{ID: 4, Name: "世界BOSS", AllowCombat: true, MaxOnline: 30, MaxLines: 2},
	})
	boss := gcruntime.MonsterDef{
		ID: 2, Name: "荒原霸主", Kind: "boss", HP: 5000, Attack: 15, Defense: 5,
		MoveSpeed: 3, AttackRange: 3, AttackIntervalMs: 1500, AggroRange: 10, LeashRange: 25,
	}
	pack := gcruntime.MonsterDef{
		ID: 3, Name: "狼群", Kind: "mob", HP: 200, Attack: 8, Defense: 2,
		MoveSpeed: 4, AttackRange: 2, AttackIntervalMs: 1200, AggroRange: 6, LeashRange: 12,
	}
	gcruntime.BuildMonsters([]gcruntime.MonsterDef{wolfDef(), boss, pack}, []gcruntime.SpawnDef{
		wildSpawn(),
		{ID: 2, Feature: "worldboss", SceneID: 4, Line: 0, MonsterID: 2, X: 20, Count: 1, RespawnMs: 300000},
		{ID: 3, Feature: "worldboss", SceneID: 4, Line: 0, MonsterID: 2, X: -20, Z: 10, Count: 1, RespawnMs: 300000},
		{ID: 4, Feature: "worldboss", SceneID: 4, Line: 0, MonsterID: 3, X: 8, Z: 8, Count: 4, RespawnMs: 15000},
	})
	SyncSpawns(nil)
	if _, ok := MonsterBySlot(1, 1, 0); !ok {
		t.Fatal("wild wolf missing")
	}
	if _, ok := MonsterBySlot(1, 2, 0); ok {
		t.Fatal("wild wolf copied to line 2")
	}
	for _, line := range []int32{1, 2} {
		if _, ok := MonsterBySlot(2, line, 0); !ok {
			t.Fatalf("boss spawn 2 line %d", line)
		}
		if _, ok := MonsterBySlot(3, line, 0); !ok {
			t.Fatalf("boss spawn 3 line %d", line)
		}
		for slot := int32(0); slot < 4; slot++ {
			uid, ok := MonsterBySlot(4, line, slot)
			if !ok {
				t.Fatalf("pack line %d slot %d", line, slot)
			}
			pose, posed := world.PoseOf(uid)
			if !posed || pose.Line != line || pose.SceneID != 4 {
				t.Fatalf("pack pose %+v", pose)
			}
		}
	}
}
