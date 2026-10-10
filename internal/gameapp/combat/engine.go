package combat

import (
	"math"
	"sort"
	"sync"

	gcruntime "github.com/example/mmo-server/gameconfig/pkg/runtime"
	"github.com/example/mmo-server/internal/code"
	"github.com/example/mmo-server/internal/gameapp/world"
	"github.com/example/mmo-server/internal/gtime"
)

// Hit 是一拍内的一次伤害、治疗或复活。
type Hit struct {
	SourceUID int64 // 造成这次变化的单位
	TargetUID int64 // 被命中的单位
	SkillID   int32 // 技能 id，怪物出手为 0
	BuffID    int32 // 持续效果 id，普通技能命中为 0
	Amount    int32 // 实际扣血或治疗量
	Heal      bool  // true 表示治疗或复活回血
	TargetHP  int32 // 结算后目标当前生命
	Dead      bool  // 这次结算后目标是否死亡
}

// Frame 是一名观众这一拍要收到的合并包。
type Frame struct {
	ViewerUID int64  // 接收这条战斗帧的玩家
	AgentPath string // 该玩家的网关连接路径
	Hits      []Hit  // 这一拍合并后的命中
	Truncated bool   // 超过单包上限，后面的命中被丢掉
}

// skillSnap 是入队时拷下的技能数值，热更不会改已经排队的这一下。
type skillSnap struct {
	id        int32  // 技能 id
	target    string // single 点名，aoe_self 自身圆心
	castRange int32  // 点名距离
	radius    int32  // 范围斩半径
	damage    int32  // 附加伤害，与攻击力系数相加
	factor    int32  // 攻击力百分比
	buffID    int32  // 命中后要挂的 Buff，0 表示不挂
}

// intent 是已经通过校验、等下一拍结算的一次出手。
type intent struct {
	uid    int64     // 施法者
	skill  skillSnap // 入队时的技能拷贝
	target int64     // 点名目标，范围技能不用
}

// buffInst 是单位身上的一条 Buff，数值在挂上时拷贝。
type buffInst struct {
	id         int32      // 配表 Buff id
	source     int64      // 施加者，持续伤害记在这个人头上
	effect     buffEffect // dot、hot、stun 或 attr
	value      int32      // 一层的跳伤、治疗或属性加成
	intervalMs int64      // 跳伤或治疗间隔
	expireAt   int64      // 到期毫秒，到点不再计入
	nextTick   int64      // 下一跳的毫秒时间
	stacks     int32      // 当前层数
	maxStack   int32      // 层数上限
	stat       statKind   // attr 加成的属性
	mode       attrMode   // flat 固定值，percent 百分比
}

// unit 是战斗里的一名玩家或一只怪物。离场即删，不落库。
type unit struct {
	hp          int32       // 当前生命
	maxHP       int32       // 最终生命上限，含属性 Buff
	attack      int32       // 最终攻击
	defense     int32       // 最终防御
	baseMaxHP   int32       // 进场或刷出时拷下的生命上限
	baseAttack  int32       // 进场或刷出时拷下的攻击
	baseDefense int32       // 进场或刷出时拷下的防御
	dead        bool        // 是否已死亡
	deadAt      int64       // 死亡时的毫秒时间
	buffs       []*buffInst // 身上还没到期的 Buff

	monster        bool               // true 表示怪物，不走玩家进场
	templateID     int32              // 怪物模板 id
	spawnID        int32              // 刷怪点 id
	slot           int32              // 同一刷怪点上的第几只
	moveSpeed      float32            // 每秒位移
	attackRange    float32            // 可以出手的距离
	attackInterval int64              // 怪物出手间隔毫秒
	aggroRange     float32            // 开始追击的距离
	leashRange     float32            // 超过则回家的距离
	respawnMs      int64              // 死亡后复活毫秒
	homeX          float32            // 出生点 X
	homeY          float32            // 出生点 Y
	homeZ          float32            // 出生点 Z
	sceneID        int32              // 所在地图
	line           int32              // 所在分线
	target         int64              // 怪物当前追击的玩家
	nextAttack     int64              // 怪物下一击的毫秒时间
	playerID       int64              // 玩家角色 id，击杀奖励发给这个角色
	lastHit        int64              // 把这只怪物打到 0 的单位
	damage         map[int64]*hurtRec // 这一条命里每个玩家的实际扣血
}

// hurtRec 是一名玩家对这只怪物这一条命造成的实际扣血。
type hurtRec struct {
	total    int32 // 这一条命的伤害合计
	at       int64 // 达到当前合计的毫秒时间
	playerID int64 // 出手时的角色 id
}

// Hurt 是结算时抄出来的一条伤害。
type Hurt struct {
	UID      int64 // 造成伤害的玩家
	PlayerID int64 // 发奖用的角色 id
	Damage   int32 // 实际扣血合计
	At       int64 // 达到该伤害的毫秒时间
}

// KillSnap 是一只怪物死亡时的伤害榜。复活前已经从单位上清掉。
type KillSnap struct {
	MonsterUID int64  // 死亡的怪物实例
	TemplateID int32  // 怪物模板，用来查击杀奖励
	SceneID    int32  // 死亡时所在地图
	Line       int32  // 死亡时所在分线
	LastHit    int64  // 最后一击的单位
	Hits       []Hurt // 这一条命的伤害榜
}

var (
	mu           sync.Mutex
	units        = map[int64]*unit{}
	cds          = map[int64]map[int32]int64{}
	queue        []intent
	pendingKills []KillSnap
	nowFn        = func() int64 { return gtime.Now().UnixMilli() }
)

// ResetForTest 清空战斗内存状态并恢复时钟。仅测试调用。
func ResetForTest() {
	mu.Lock()
	defer mu.Unlock()
	units = map[int64]*unit{}
	cds = map[int64]map[int32]int64{}
	queue = nil
	pendingKills = nil
	nowFn = func() int64 { return gtime.Now().UnixMilli() }
	storeMonsterSteps(nil)
}

// SetClockForTest 固定当前毫秒，便于步进心跳。仅测试调用。
func SetClockForTest(now int64) {
	mu.Lock()
	defer mu.Unlock()
	nowFn = func() int64 { return now }
}

// Enter 进场满血，并清掉该单位的冷却与 Buff。最大生命读配表。
func Enter(uid int64) {
	if uid < 1 {
		return
	}
	c, ok := gcruntime.CombatConstRow()
	if !ok || c.MaxHP < 1 {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	units[uid] = &unit{
		hp: c.MaxHP, maxHP: c.MaxHP, baseMaxHP: c.MaxHP,
		attack: c.Attack, baseAttack: c.Attack,
		defense: c.Defense, baseDefense: c.Defense,
	}
	delete(cds, uid)
}

// SetPlayer 记下这个 uid 当前进场的角色。击杀奖励发给这个角色。
func SetPlayer(uid, playerID int64) {
	if uid < 1 || playerID < 1 {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	if u := units[uid]; u != nil && !u.monster {
		u.playerID = playerID
	}
}

// TakeKills 取出上一拍死亡怪物的伤害榜。
func TakeKills() []KillSnap {
	mu.Lock()
	defer mu.Unlock()
	out := pendingKills
	pendingKills = nil
	return out
}

// Leave 离场时丢掉生命、冷却、Buff 和尚未结算的技能。
func Leave(uid int64) {
	mu.Lock()
	defer mu.Unlock()
	delete(units, uid)
	delete(cds, uid)
	kept := make([]intent, 0, len(queue))
	for _, it := range queue {
		if it.uid != uid {
			kept = append(kept, it)
		}
	}
	queue = kept
}

// Snapshot 只读当前生命。未进战斗时 ok 为 false。
func Snapshot(uid int64) (hp, maxHP int32, alive, ok bool) {
	mu.Lock()
	defer mu.Unlock()
	u, found := units[uid]
	if !found {
		return 0, 0, false, false
	}
	return u.hp, u.maxHP, !u.dead, true
}

// Cast 接受技能意图。非法技能、禁手、冷却、距离和死亡立刻返回；扣血在下一拍。
func Cast(uid int64, skillID int32, targetUID int64) int32 {
	sk, ok := gcruntime.Skill(skillID)
	if !ok || (sk.Target != "single" && sk.Target != "aoe_self") {
		return code.CombatSkillInvalid
	}
	mu.Lock()
	defer mu.Unlock()
	u := units[uid]
	if u == nil {
		return code.PlayerNotEntered
	}
	if u.dead {
		return code.CombatSelfDead
	}
	now := nowFn()
	if stunned(u, now) {
		return code.CombatStunned
	}
	if !sceneAllowsCombat(uid) {
		return code.SceneCombatDisabled
	}
	if ready, exists := cds[uid][skillID]; exists && now < ready {
		return code.CombatCooldown
	}
	if sk.Target == "single" {
		if targetUID < 1 || targetUID == uid {
			return code.CombatTargetInvalid
		}
		tgt := units[targetUID]
		if tgt == nil {
			return code.CombatTargetInvalid
		}
		if tgt.dead {
			return code.CombatTargetDead
		}
		if !inRange(uid, targetUID, sk.CastRange) {
			return code.CombatOutOfRange
		}
	}
	if cds[uid] == nil {
		cds[uid] = map[int32]int64{}
	}
	cds[uid][skillID] = now + int64(sk.CooldownMs)
	queue = append(queue, intent{
		uid: uid,
		skill: skillSnap{
			id: sk.ID, target: sk.Target, castRange: sk.CastRange,
			radius: sk.Radius, damage: sk.Damage, factor: sk.Factor, buffID: sk.BuffID,
		},
		target: targetUID,
	})
	return code.OK
}

// ApplyBuff 按当前配表给单位挂 Buff，并拷贝当时的数值。
func ApplyBuff(uid int64, buffID int32) int32 {
	def, ok := gcruntime.Buff(buffID)
	if !ok || !validBuff(def) {
		return code.CombatBuffInvalid
	}
	mu.Lock()
	defer mu.Unlock()
	u := units[uid]
	if u == nil {
		return code.PlayerNotEntered
	}
	if u.dead {
		return code.CombatSelfDead
	}
	now := nowFn()
	if stunned(u, now) {
		return code.CombatStunned
	}
	applyBuff(u, uid, def, now)
	return code.OK
}

// Tick 结算队列、Buff、怪物行动和复活，并按观众合成广播包。
// 怪物位移在放开战斗锁之后才写入房间。
func Tick() []Frame {
	mu.Lock()
	now := nowFn()
	pending := queue
	queue = nil
	var hits []Hit
	for _, it := range pending {
		hits = append(hits, resolveIntent(it, now)...)
	}
	hits = append(hits, tickBuffs(now)...)
	hits = append(hits, tickRespawn(now)...)
	steps, attacks := tickMonsters(now)
	hits = append(hits, attacks...)
	capN := 64
	if c, ok := gcruntime.CombatConstRow(); ok && c.FrameEventCap > 0 {
		capN = int(c.FrameEventCap)
	}
	frames := buildFrames(hits, capN)
	mu.Unlock()
	for _, step := range steps {
		world.SetPosition(step.UID, step.X, step.Y, step.Z)
	}
	storeMonsterSteps(steps)
	return frames
}

func validBuff(def gcruntime.BuffDef) bool {
	if def.DurationMs < 1 {
		return false
	}
	switch buffEffect(def.Effect) {
	case effectDot, effectHot:
		return def.Value > 0
	case effectStun:
		return true
	case effectAttr:
		st, ok := gcruntime.StatByName(def.Stat)
		if !ok || !st.Settle {
			return false
		}
		switch attrMode(def.Mode) {
		case modeFlat:
			return st.AllowFlat
		case modePercent:
			return st.AllowPercent
		default:
			return false
		}
	default:
		return false
	}
}

// stunned 表示身上还有未到期的禁手。now 由调用方在持锁时取。
func stunned(u *unit, now int64) bool {
	if u == nil {
		return false
	}
	for _, b := range u.buffs {
		if b != nil && b.effect == effectStun && now < b.expireAt {
			return true
		}
	}
	return false
}

func resolveIntent(it intent, now int64) []Hit {
	u := units[it.uid]
	if u == nil || u.dead || stunned(u, now) || !sceneAllowsCombat(it.uid) {
		return nil
	}
	switch it.skill.target {
	case "single":
		return hitOne(it.uid, it.target, it.skill, now)
	case "aoe_self":
		return hitAOE(it.uid, it.skill, now)
	default:
		return nil
	}
}

func hitOne(src, dst int64, sk skillSnap, now int64) []Hit {
	tgt := units[dst]
	if tgt == nil || tgt.dead {
		return nil
	}
	if !inRange(src, dst, sk.castRange) {
		return nil
	}
	return []Hit{applyHit(src, dst, tgt, sk, now)}
}

func hitAOE(src int64, sk skillSnap, now int64) []Hit {
	sp, ok := world.PoseOf(src)
	if !ok {
		return nil
	}
	ids := make([]int64, 0, len(units))
	for uid := range units {
		ids = append(ids, uid)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	var hits []Hit
	for _, uid := range ids {
		if uid == src {
			continue
		}
		tgt := units[uid]
		if tgt == nil || tgt.dead {
			continue
		}
		tp, ok := world.PoseOf(uid)
		if !ok || tp.SceneID != sp.SceneID || tp.Line != sp.Line {
			continue
		}
		if distance(sp.X, sp.Z, tp.X, tp.Z) > float32(sk.radius) {
			continue
		}
		hits = append(hits, applyHit(src, uid, tgt, sk, now))
	}
	return hits
}

func applyHit(src, dst int64, tgt *unit, sk skillSnap, now int64) Hit {
	dmg := strikeDamage(units[src], sk.damage, sk.factor, tgt, now)
	applyDamage(tgt, dst, src, dmg, now)
	h := Hit{SourceUID: src, TargetUID: dst, SkillID: sk.id, Amount: dmg, TargetHP: tgt.hp, Dead: tgt.dead}
	if !tgt.dead && sk.buffID > 0 {
		if def, ok := gcruntime.Buff(sk.buffID); ok && validBuff(def) {
			applyBuff(tgt, src, def, now)
		}
	}
	return h
}

func applyBuff(u *unit, source int64, def gcruntime.BuffDef, now int64) {
	maxStack := def.MaxStack
	if maxStack < 1 {
		maxStack = 1
	}
	interval := int64(def.IntervalMs)
	if interval < 1 {
		interval = int64(def.DurationMs)
	}
	for _, b := range u.buffs {
		if b.id != def.ID {
			continue
		}
		if b.stacks < maxStack {
			b.stacks++
		}
		b.source = source
		b.effect = buffEffect(def.Effect)
		b.value = def.Value
		b.intervalMs = interval
		b.maxStack = maxStack
		b.expireAt = now + int64(def.DurationMs)
		b.stat = statKind(def.Stat)
		b.mode = attrMode(def.Mode)
		refreshAttrs(u, now)
		return
	}
	u.buffs = append(u.buffs, &buffInst{
		id: def.ID, source: source, effect: buffEffect(def.Effect), value: def.Value,
		intervalMs: interval, expireAt: now + int64(def.DurationMs),
		nextTick: now + interval, stacks: 1, maxStack: maxStack,
		stat: statKind(def.Stat), mode: attrMode(def.Mode),
	})
	refreshAttrs(u, now)
}

// applyDamage 扣血。实际扣掉的数量不超过剩余生命。怪物被玩家打到 0 时记下伤害榜。
func applyDamage(u *unit, uid, source int64, amount int32, now int64) {
	if u == nil || u.dead || amount < 1 {
		return
	}
	dealt := amount
	if dealt > u.hp {
		dealt = u.hp
	}
	u.hp -= dealt
	killed := false
	if u.hp <= 0 {
		u.hp = 0
		u.dead = true
		u.deadAt = now
		u.buffs = nil
		u.target = 0
		killed = true
		u.lastHit = source
		resetBaseStats(u)
	}
	if u.monster && dealt > 0 && playerSource(source) {
		addHurt(u, source, dealt, now)
	}
	if killed && u.monster {
		pendingKills = append(pendingKills, snapKill(uid, u))
		u.damage = nil
		u.lastHit = 0
	}
}

// playerSource 判断这次伤害算不算玩家。怪物打人、怪打怪不进击杀榜。
// 来源单位已经离场时仍算玩家，持续伤害记在施加 Buff 的人身上。
func playerSource(source int64) bool {
	if source < 1 {
		return false
	}
	src := units[source]
	return src == nil || !src.monster
}

// addHurt 累加这名玩家对这只怪这一条命的实际扣血。at 是达到当前伤害合计的时间。
func addHurt(u *unit, source int64, dealt int32, now int64) {
	if u.damage == nil {
		u.damage = map[int64]*hurtRec{}
	}
	rec := u.damage[source]
	if rec == nil {
		rec = &hurtRec{}
		u.damage[source] = rec
	}
	rec.total += dealt
	rec.at = now
	if src := units[source]; src != nil {
		rec.playerID = src.playerID
	}
}

// snapKill 抄下死亡瞬间的伤害榜。调用方随后清空单位上的记账，复活从零开始。
func snapKill(uid int64, u *unit) KillSnap {
	hits := make([]Hurt, 0, len(u.damage))
	for id, rec := range u.damage {
		if rec == nil || rec.total < 1 {
			continue
		}
		hits = append(hits, Hurt{UID: id, PlayerID: rec.playerID, Damage: rec.total, At: rec.at})
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].UID < hits[j].UID })
	return KillSnap{
		MonsterUID: uid, TemplateID: u.templateID, SceneID: u.sceneID, Line: u.line,
		LastHit: u.lastHit, Hits: hits,
	}
}

func applyHeal(u *unit, amount int32) {
	if u == nil || u.dead || amount < 1 {
		return
	}
	u.hp += amount
	if u.hp > u.maxHP {
		u.hp = u.maxHP
	}
}

func tickBuffs(now int64) []Hit {
	ids := make([]int64, 0, len(units))
	for uid := range units {
		ids = append(ids, uid)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	var hits []Hit
	for _, uid := range ids {
		u := units[uid]
		if u == nil || u.dead {
			continue
		}
		kept := make([]*buffInst, 0, len(u.buffs))
		for _, b := range u.buffs {
			if u.dead || now >= b.expireAt {
				continue
			}
			if now >= b.nextTick && b.effect != effectStun {
				amt := b.value * b.stacks
				if amt < 1 {
					amt = b.value
				}
				if b.effect == effectHot {
					before := u.hp
					applyHeal(u, amt)
					got := u.hp - before
					hits = append(hits, Hit{SourceUID: b.source, TargetUID: uid, BuffID: b.id, Amount: got, Heal: true, TargetHP: u.hp})
				} else {
					applyDamage(u, uid, b.source, amt, now)
					hits = append(hits, Hit{SourceUID: b.source, TargetUID: uid, BuffID: b.id, Amount: amt, TargetHP: u.hp, Dead: u.dead})
				}
				b.nextTick += b.intervalMs
			}
			if !u.dead {
				kept = append(kept, b)
			}
		}
		if u.dead {
			u.buffs = nil
			resetBaseStats(u)
		} else {
			u.buffs = kept
			refreshAttrs(u, now)
		}
	}
	return hits
}

func tickRespawn(now int64) []Hit {
	c, ok := gcruntime.CombatConstRow()
	if !ok || c.RespawnMs < 1 {
		return nil
	}
	ids := make([]int64, 0, len(units))
	for uid := range units {
		ids = append(ids, uid)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	var hits []Hit
	for _, uid := range ids {
		u := units[uid]
		if u == nil || !u.dead || u.monster {
			continue
		}
		if now-u.deadAt < int64(c.RespawnMs) {
			continue
		}
		u.dead = false
		u.buffs = nil
		resetBaseStats(u)
		u.hp = u.maxHP
		hits = append(hits, Hit{SourceUID: uid, TargetUID: uid, Amount: u.maxHP, Heal: true, TargetHP: u.hp})
	}
	return hits
}

func inRange(a, b int64, dist int32) bool {
	pa, oka := world.PoseOf(a)
	pb, okb := world.PoseOf(b)
	if !oka || !okb || pa.SceneID != pb.SceneID || pa.Line != pb.Line {
		return false
	}
	return distance(pa.X, pa.Z, pb.X, pb.Z) <= float32(dist)
}

func distance(x1, z1, x2, z2 float32) float32 {
	return float32(math.Hypot(float64(x1-x2), float64(z1-z2)))
}

func buildFrames(hits []Hit, capN int) []Frame {
	if len(hits) == 0 {
		return nil
	}
	if capN < 1 {
		capN = 64
	}
	ids := make([]int64, 0, len(units))
	for uid := range units {
		ids = append(ids, uid)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	var frames []Frame
	for _, uid := range ids {
		if u := units[uid]; u == nil || u.monster {
			continue
		}
		viewer, ok := world.PoseOf(uid)
		if !ok {
			continue
		}
		mine := make([]Hit, 0)
		for _, h := range hits {
			target, okT := world.PoseOf(h.TargetUID)
			if !okT || target.SceneID != viewer.SceneID || target.Line != viewer.Line {
				continue
			}
			see := world.InAOI(viewer.X, viewer.Z, target.X, target.Z)
			if source, okS := world.PoseOf(h.SourceUID); okS && source.SceneID == viewer.SceneID && source.Line == viewer.Line && world.InAOI(viewer.X, viewer.Z, source.X, source.Z) {
				see = true
			}
			if see {
				mine = append(mine, h)
			}
		}
		if len(mine) == 0 {
			continue
		}
		sort.SliceStable(mine, func(i, j int) bool {
			return hitRank(uid, mine[i]) < hitRank(uid, mine[j])
		})
		truncated := false
		if len(mine) > capN {
			mine = mine[:capN]
			truncated = true
		}
		frames = append(frames, Frame{ViewerUID: uid, AgentPath: viewer.AgentPath, Hits: mine, Truncated: truncated})
	}
	return frames
}

func sceneAllowsCombat(uid int64) bool {
	pose, ok := world.PoseOf(uid)
	if !ok {
		return false
	}
	return gcruntime.SceneAllowsCombat(pose.SceneID)
}

func hitRank(viewer int64, h Hit) int {
	if h.SourceUID == viewer || h.TargetUID == viewer {
		return 0
	}
	return 1
}
