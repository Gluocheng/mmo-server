package importdata

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/example/mmo-server/gameconfig/pkg/schema"
)

const (
	// SkillTableFile 是技能表 JSON 文件名（与 Luban 导出命名一致）。
	SkillTableFile = "skill_tbskill.json"
	// BuffTableFile 是 Buff 表 JSON 文件名。
	BuffTableFile = "buff_tbbuff.json"
	// CombatConstTableFile 是战斗常量表 JSON 文件名。
	CombatConstTableFile = "combat_const_tbcombatconst.json"
)

type skillJSON struct {
	ID         int32  `json:"id"`
	Name       string `json:"name"`
	Target     string `json:"target"`
	CastRange  int32  `json:"cast_range"`
	Radius     int32  `json:"radius"`
	CooldownMs int32  `json:"cooldown_ms"`
	Damage     int32  `json:"damage"`
	BuffID     int32  `json:"buff_id"`
}

type buffJSON struct {
	ID         int32  `json:"id"`
	Name       string `json:"name"`
	DurationMs int32  `json:"duration_ms"`
	IntervalMs int32  `json:"interval_ms"`
	Effect     string `json:"effect"`
	Value      int32  `json:"value"`
	MaxStack   int32  `json:"max_stack"`
}

type constJSON struct {
	ID            int32 `json:"id"`
	MaxHP         int32 `json:"max_hp"`
	TickMs        int32 `json:"tick_ms"`
	RespawnMs     int32 `json:"respawn_ms"`
	FrameEventCap int32 `json:"frame_event_cap"`
}

// LoadSkillsFromJSONFile 读取技能 JSON 数组。
func LoadSkillsFromJSONFile(path string) ([]schema.CfgSkill, error) {
	var rows []skillJSON
	if err := readJSONArray(path, &rows); err != nil {
		return nil, err
	}
	out := make([]schema.CfgSkill, 0, len(rows))
	for _, r := range rows {
		if r.ID < 1 {
			return nil, fmt.Errorf("invalid skill row in %s", path)
		}
		out = append(out, schema.CfgSkill{
			ID: r.ID, Name: r.Name, Target: r.Target, CastRange: r.CastRange,
			Radius: r.Radius, CooldownMs: r.CooldownMs, Damage: r.Damage, BuffID: r.BuffID,
		})
	}
	return out, nil
}

// LoadBuffsFromJSONFile 读取 Buff JSON 数组。
func LoadBuffsFromJSONFile(path string) ([]schema.CfgBuff, error) {
	var rows []buffJSON
	if err := readJSONArray(path, &rows); err != nil {
		return nil, err
	}
	out := make([]schema.CfgBuff, 0, len(rows))
	for _, r := range rows {
		if r.ID < 1 {
			return nil, fmt.Errorf("invalid buff row in %s", path)
		}
		out = append(out, schema.CfgBuff{
			ID: r.ID, Name: r.Name, DurationMs: r.DurationMs, IntervalMs: r.IntervalMs,
			Effect: r.Effect, Value: r.Value, MaxStack: r.MaxStack,
		})
	}
	return out, nil
}

// LoadCombatConstFromJSONFile 读取战斗常量 JSON 数组，取 id=1 的行。
func LoadCombatConstFromJSONFile(path string) (schema.CfgCombatConst, error) {
	var rows []constJSON
	if err := readJSONArray(path, &rows); err != nil {
		return schema.CfgCombatConst{}, err
	}
	for _, r := range rows {
		if r.ID == 1 {
			return schema.CfgCombatConst{
				ID: 1, MaxHP: r.MaxHP, TickMs: r.TickMs, RespawnMs: r.RespawnMs, FrameEventCap: r.FrameEventCap,
			}, nil
		}
	}
	return schema.CfgCombatConst{}, fmt.Errorf("combat const id=1 missing in %s", path)
}

func readJSONArray(path string, dest any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, dest); err != nil {
		return fmt.Errorf("parse json %s: %w", path, err)
	}
	return nil
}
