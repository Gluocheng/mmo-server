package combat

import (
	"sort"
	"strings"
	"sync"

	cherrySnowflake "github.com/cherry-game/cherry/extend/snowflake"
	cfacade "github.com/cherry-game/cherry/facade"
	gcruntime "github.com/example/mmo-server/gameconfig/pkg/runtime"
	"github.com/example/mmo-server/internal/gameapp/world"
)

// MonsterStep 是这一拍怪物要落到的新坐标。战斗锁放开后才写入房间。
type MonsterStep struct {
	UID int64   // 怪物实例
	X   float32 // 新坐标 X
	Y   float32 // 新坐标 Y
	Z   float32 // 新坐标 Z
}

// slotKey 标识一个刷怪点在某条线上的一个槽位。
type slotKey struct {
	spawnID int32 // 刷怪点 id
	line    int32 // 分线
	slot    int32 // 该点上的第几只，从 0 起
}

// slotSpec 是这个槽位应刷出的怪物。已在场且这些值没变就保留原实例。
type slotSpec struct {
	spawnID   int32                // 刷怪点 id
	slot      int32                // 槽位序号
	monsterID int32                // 怪物模板 id
	sceneID   int32                // 地图 id
	line      int32                // 分线。配表 line=0 时已展开成具体线
	x         float32              // 出生点 X，同点多只沿 X 每隔 2 格
	y         float32              // 出生点 Y
	z         float32              // 出生点 Z
	respawnMs int32                // 死亡后复活毫秒
	def       gcruntime.MonsterDef // 刷出时拷贝的模板
}

var (
	stepMu       sync.Mutex
	monsterSteps []MonsterStep // 上一拍算出、等战斗锁放开后写入房间的位移
)

func storeMonsterSteps(steps []MonsterStep) {
	stepMu.Lock()
	monsterSteps = steps
	stepMu.Unlock()
}

// TakeMonsterSteps 取出上一拍尚未广播的怪物位移。
func TakeMonsterSteps() []MonsterStep {
	stepMu.Lock()
	defer stepMu.Unlock()
	out := monsterSteps
	monsterSteps = nil
	return out
}

// SyncSpawns 按当前刷怪点生成、保留或移除怪物。已在场且出生信息没变的实例保持生命和 uid。
func SyncSpawns(sender cfacade.IActor) {
	specs := desiredSlots()
	mu.Lock()
	var gone []int64
	for uid, u := range units {
		if u == nil || !u.monster {
			continue
		}
		key := slotKey{spawnID: u.spawnID, line: u.line, slot: u.slot}
		spec, ok := specs[key]
		if !ok || !sameSlot(u, spec) {
			gone = append(gone, uid)
			continue
		}
		delete(specs, key)
	}
	mu.Unlock()
	for _, uid := range gone {
		Leave(uid)
		world.DropMonster(sender, uid)
	}
	keys := make([]slotKey, 0, len(specs))
	for key := range specs {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].spawnID != keys[j].spawnID {
			return keys[i].spawnID < keys[j].spawnID
		}
		if keys[i].line != keys[j].line {
			return keys[i].line < keys[j].line
		}
		return keys[i].slot < keys[j].slot
	})
	for _, key := range keys {
		spec := specs[key]
		uid := cherrySnowflake.NextID()
		world.PlaceMonster(uid, spec.sceneID, spec.line, spec.x, spec.y, spec.z, spec.monsterID)
		mu.Lock()
		units[uid] = unitFromSpec(spec)
		mu.Unlock()
		world.AnnounceMonster(sender, uid)
	}
}

// MonsterBySlot 返回某个刷怪点、某条线上槽位的实例 uid。
func MonsterBySlot(spawnID, line, slot int32) (int64, bool) {
	mu.Lock()
	defer mu.Unlock()
	for uid, u := range units {
		if u != nil && u.monster && u.spawnID == spawnID && u.line == line && u.slot == slot {
			return uid, true
		}
	}
	return 0, false
}

// desiredSlots 按当前配表算出每条线、每个槽位应有的怪物。line=0 会展开到这张图的每一条线。
func desiredSlots() map[slotKey]slotSpec {
	out := make(map[slotKey]slotSpec)
	for _, sp := range gcruntime.Spawns() {
		if sp.ID < 1 || strings.TrimSpace(sp.Feature) == "" || sp.Count < 1 || sp.Count > 4 || sp.RespawnMs < 1000 {
			continue
		}
		mon, ok := gcruntime.Monster(sp.MonsterID)
		if !ok || !usableMonster(mon) {
			continue
		}
		scene, ok := gcruntime.Scene(sp.SceneID)
		if !ok || !scene.AllowCombat {
			continue
		}
		lines := spawnLines(sp.Line, scene.LineCount())
		for _, line := range lines {
			for i := int32(0); i < sp.Count; i++ {
				out[slotKey{spawnID: sp.ID, line: line, slot: i}] = slotSpec{
					spawnID: sp.ID, slot: i, monsterID: mon.ID,
					sceneID: sp.SceneID, line: line,
					x: sp.X + float32(i)*2, y: sp.Y, z: sp.Z,
					respawnMs: sp.RespawnMs, def: mon,
				}
			}
		}
	}
	return out
}

// spawnLines 把 line=0 展开成这张图的每一条线。写了具体线号则只刷那一条。
func spawnLines(line, lineCount int32) []int32 {
	if lineCount < 1 {
		return nil
	}
	if line == 0 {
		out := make([]int32, 0, lineCount)
		for i := int32(1); i <= lineCount; i++ {
			out = append(out, i)
		}
		return out
	}
	if line < 1 || line > lineCount {
		return nil
	}
	return []int32{line}
}

func usableMonster(def gcruntime.MonsterDef) bool {
	if def.ID < 1 || def.HP < 1 || def.Attack < 1 || def.AttackRange <= 0 || def.AttackIntervalMs < 100 {
		return false
	}
	if def.Defense < 0 || def.MoveSpeed < 0 {
		return false
	}
	if def.AggroRange < def.AttackRange || def.LeashRange <= def.AggroRange {
		return false
	}
	return def.Kind == "boss" || def.Kind == "mob"
}

func sameSlot(u *unit, spec slotSpec) bool {
	return u.templateID == spec.monsterID && u.sceneID == spec.sceneID && u.line == spec.line &&
		u.homeX == spec.x && u.homeY == spec.y && u.homeZ == spec.z && u.respawnMs == int64(spec.respawnMs)
}

func unitFromSpec(spec slotSpec) *unit {
	return &unit{
		hp: spec.def.HP, maxHP: spec.def.HP, baseMaxHP: spec.def.HP,
		defense: spec.def.Defense, baseDefense: spec.def.Defense,
		monster: true, templateID: spec.monsterID, spawnID: spec.spawnID, slot: spec.slot,
		attack: spec.def.Attack, baseAttack: spec.def.Attack, moveSpeed: spec.def.MoveSpeed, attackRange: spec.def.AttackRange,
		attackInterval: int64(spec.def.AttackIntervalMs), aggroRange: spec.def.AggroRange,
		leashRange: spec.def.LeashRange, respawnMs: int64(spec.respawnMs),
		homeX: spec.x, homeY: spec.y, homeZ: spec.z, sceneID: spec.sceneID, line: spec.line,
	}
}

func tickMonsters(now int64) ([]MonsterStep, []Hit) {
	var steps []MonsterStep
	var hits []Hit
	for _, uid := range monsterIDs() {
		u := units[uid]
		if u == nil || !u.monster {
			continue
		}
		if u.dead {
			if u.respawnMs < 1 || now-u.deadAt < u.respawnMs {
				continue
			}
			u.dead = false
			u.buffs = nil
			resetBaseStats(u)
			u.hp = u.maxHP
			u.damage = nil
			u.lastHit = 0
			u.target = 0
			u.nextAttack = 0
			hits = append(hits, Hit{SourceUID: uid, TargetUID: uid, Amount: u.maxHP, Heal: true, TargetHP: u.hp})
			steps = append(steps, MonsterStep{UID: uid, X: u.homeX, Y: u.homeY, Z: u.homeZ})
			continue
		}
		if stunned(u, now) {
			continue
		}
		pos, ok := world.PoseOf(uid)
		if !ok {
			continue
		}
		if distance(pos.X, pos.Z, u.homeX, u.homeZ) > u.leashRange {
			u.target = 0
			u.nextAttack = 0
			if step, moved := walkTo(pos.X, pos.Z, u.homeX, u.homeZ, stepLen(u)); moved {
				steps = append(steps, MonsterStep{UID: uid, X: step.x, Y: u.homeY, Z: step.z})
			}
			continue
		}
		target, dist := nearestPlayer(u, pos)
		u.target = target
		if target == 0 {
			u.nextAttack = 0
			if step, moved := walkTo(pos.X, pos.Z, u.homeX, u.homeZ, stepLen(u)); moved {
				steps = append(steps, MonsterStep{UID: uid, X: step.x, Y: u.homeY, Z: step.z})
			}
			continue
		}
		if dist > u.attackRange {
			u.nextAttack = 0
			tp, ok := world.PoseOf(target)
			if !ok {
				continue
			}
			if step, moved := walkTo(pos.X, pos.Z, tp.X, tp.Z, stepLen(u)); moved {
				steps = append(steps, MonsterStep{UID: uid, X: step.x, Y: u.homeY, Z: step.z})
			}
			continue
		}
		if u.nextAttack > now {
			continue
		}
		foe := units[target]
		if foe == nil || foe.dead {
			continue
		}
		dmg := strikeDamage(u, 0, 100, foe, now)
		applyDamage(foe, target, uid, dmg, now)
		hits = append(hits, Hit{SourceUID: uid, TargetUID: target, Amount: dmg, TargetHP: foe.hp, Dead: foe.dead})
		u.nextAttack = now + u.attackInterval
	}
	return steps, hits
}

func monsterIDs() []int64 {
	ids := make([]int64, 0)
	for uid, u := range units {
		if u != nil && u.monster {
			ids = append(ids, uid)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func nearestPlayer(self *unit, pos world.Pose) (int64, float32) {
	var best int64
	var bestDist float32
	found := false
	for uid, u := range units {
		if u == nil || u.monster || u.dead {
			continue
		}
		p, ok := world.PoseOf(uid)
		if !ok || p.SceneID != self.sceneID || p.Line != self.line {
			continue
		}
		d := distance(pos.X, pos.Z, p.X, p.Z)
		if d > self.aggroRange {
			continue
		}
		if !found || d < bestDist || (d == bestDist && uid < best) {
			found = true
			best = uid
			bestDist = d
		}
	}
	if !found {
		return 0, 0
	}
	return best, bestDist
}

type xz struct {
	x, z float32
}

func walkTo(x, z, tx, tz, step float32) (xz, bool) {
	dist := distance(x, z, tx, tz)
	if step <= 0 || dist <= 0 {
		return xz{x, z}, false
	}
	if dist <= step {
		return xz{tx, tz}, x != tx || z != tz
	}
	scale := step / dist
	return xz{x + (tx-x)*scale, z + (tz-z)*scale}, true
}

func stepLen(u *unit) float32 {
	ms := int32(100)
	if c, ok := gcruntime.CombatConstRow(); ok && c.TickMs > 0 {
		ms = c.TickMs
	}
	return u.moveSpeed * float32(ms) / 1000
}
