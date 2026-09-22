package runtime

import "github.com/example/mmo-server/gameconfig/pkg/schema"

// SkillDef 技能配置视图。Target 为 single 或 aoe_self。
type SkillDef struct {
	ID         int32
	Name       string
	Target     string
	CastRange  int32
	Radius     int32
	CooldownMs int32
	Damage     int32
	BuffID     int32
}

// BuffDef 持续效果视图。Effect 为 dot 或 hot。
type BuffDef struct {
	ID         int32
	Name       string
	DurationMs int32
	IntervalMs int32
	Effect     string
	Value      int32
	MaxStack   int32
}

// CombatConst 战斗常量。Has 为 false 表示表里没有 id=1。
type CombatConst struct {
	MaxHP         int32
	TickMs        int32
	RespawnMs     int32
	FrameEventCap int32
}

// Skill 按 id 读取技能。
func Skill(id int32) (SkillDef, bool) {
	s := getSnapshot()
	if s == nil || s.tables == nil || s.tables.skills == nil {
		return SkillDef{}, false
	}
	def, ok := s.tables.skills[id]
	return def, ok
}

// Buff 按 id 读取 Buff。
func Buff(id int32) (BuffDef, bool) {
	s := getSnapshot()
	if s == nil || s.tables == nil || s.tables.buffs == nil {
		return BuffDef{}, false
	}
	def, ok := s.tables.buffs[id]
	return def, ok
}

// CombatConstRow 读取战斗常量。未加载 id=1 时 ok 为 false。
func CombatConstRow() (CombatConst, bool) {
	s := getSnapshot()
	if s == nil || s.tables == nil || !s.tables.hasConst {
		return CombatConst{}, false
	}
	return s.tables.combatConst, true
}

// BuildCombat 用内存行替换技能、Buff 和常量，保留已有道具表。仅测试或启动前灌入。
func BuildCombat(skills []SkillDef, buffs []BuffDef, c CombatConst) {
	s := getSnapshot()
	var base *tables
	ver := int64(0)
	tc := int32(0)
	if s != nil {
		ver = s.version
		tc = s.tableCount
		base = s.tables.clone()
	} else {
		base = &tables{}
	}
	sm := make(map[int32]SkillDef, len(skills))
	for _, sk := range skills {
		sm[sk.ID] = sk
	}
	bm := make(map[int32]BuffDef, len(buffs))
	for _, b := range buffs {
		bm[b.ID] = b
	}
	base.skills = sm
	base.buffs = bm
	base.combatConst = c
	base.hasConst = true
	swapSnapshot(&snapshot{version: ver, tableCount: tc, tables: base})
}

func skillsFromSchema(rows []schema.CfgSkill) map[int32]SkillDef {
	m := make(map[int32]SkillDef, len(rows))
	for _, r := range rows {
		m[r.ID] = SkillDef{
			ID: r.ID, Name: r.Name, Target: r.Target, CastRange: r.CastRange,
			Radius: r.Radius, CooldownMs: r.CooldownMs, Damage: r.Damage, BuffID: r.BuffID,
		}
	}
	return m
}

func buffsFromSchema(rows []schema.CfgBuff) map[int32]BuffDef {
	m := make(map[int32]BuffDef, len(rows))
	for _, r := range rows {
		m[r.ID] = BuffDef{
			ID: r.ID, Name: r.Name, DurationMs: r.DurationMs, IntervalMs: r.IntervalMs,
			Effect: r.Effect, Value: r.Value, MaxStack: r.MaxStack,
		}
	}
	return m
}

func constFromSchema(rows []schema.CfgCombatConst) CombatConst {
	for _, r := range rows {
		if r.ID == 1 {
			return CombatConst{MaxHP: r.MaxHP, TickMs: r.TickMs, RespawnMs: r.RespawnMs, FrameEventCap: r.FrameEventCap}
		}
	}
	if len(rows) > 0 {
		r := rows[0]
		return CombatConst{MaxHP: r.MaxHP, TickMs: r.TickMs, RespawnMs: r.RespawnMs, FrameEventCap: r.FrameEventCap}
	}
	return CombatConst{}
}
