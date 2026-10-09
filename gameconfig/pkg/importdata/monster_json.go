package importdata

import (
	"fmt"

	"github.com/example/mmo-server/gameconfig/pkg/schema"
)

const (
	// MonsterTableFile 是怪物模板 JSON 文件名。
	MonsterTableFile = "monster_tbmonster.json"
	// SpawnTableFile 是刷怪点 JSON 文件名。
	SpawnTableFile = "spawn_tbspawn.json"
)

type monsterJSON struct {
	ID               int32   `json:"id"`
	Name             string  `json:"name"`
	HP               int32   `json:"hp"`
	Attack           int32   `json:"attack"`
	Defense          int32   `json:"defense"`
	MoveSpeed        float32 `json:"move_speed"`
	AttackRange      float32 `json:"attack_range"`
	AttackIntervalMs int32   `json:"attack_interval_ms"`
	AggroRange       float32 `json:"aggro_range"`
	LeashRange       float32 `json:"leash_range"`
	Kind             string  `json:"kind"`
}

type spawnJSON struct {
	ID        int32   `json:"id"`
	Feature   string  `json:"feature"`
	SceneID   int32   `json:"scene_id"`
	Line      int32   `json:"line"`
	MonsterID int32   `json:"monster_id"`
	X         float32 `json:"x"`
	Y         float32 `json:"y"`
	Z         float32 `json:"z"`
	Count     int32   `json:"count"`
	RespawnMs int32   `json:"respawn_ms"`
}

// LoadMonstersFromJSONFile 读取怪物模板 JSON 数组。
func LoadMonstersFromJSONFile(path string) ([]schema.CfgMonster, error) {
	var rows []monsterJSON
	if err := readJSONArray(path, &rows); err != nil {
		return nil, err
	}
	out := make([]schema.CfgMonster, 0, len(rows))
	for _, r := range rows {
		if r.ID < 1 {
			return nil, fmt.Errorf("invalid monster row in %s", path)
		}
		out = append(out, schema.CfgMonster{
			ID: r.ID, Name: r.Name, HP: r.HP, Attack: r.Attack, Defense: r.Defense,
			MoveSpeed: r.MoveSpeed, AttackRange: r.AttackRange, AttackIntervalMs: r.AttackIntervalMs,
			AggroRange: r.AggroRange, LeashRange: r.LeashRange, Kind: r.Kind,
		})
	}
	return out, nil
}

// LoadSpawnsFromJSONFile 读取刷怪点 JSON 数组。
func LoadSpawnsFromJSONFile(path string) ([]schema.CfgSpawn, error) {
	var rows []spawnJSON
	if err := readJSONArray(path, &rows); err != nil {
		return nil, err
	}
	out := make([]schema.CfgSpawn, 0, len(rows))
	for _, r := range rows {
		if r.ID < 1 {
			return nil, fmt.Errorf("invalid spawn row in %s", path)
		}
		out = append(out, schema.CfgSpawn{
			ID: r.ID, Feature: r.Feature, SceneID: r.SceneID, Line: r.Line, MonsterID: r.MonsterID,
			X: r.X, Y: r.Y, Z: r.Z, Count: r.Count, RespawnMs: r.RespawnMs,
		})
	}
	return out, nil
}

// KillRewardTableFile 是击杀奖励 JSON 文件名。
const KillRewardTableFile = "killreward_tbkillreward.json"

type killRewardJSON struct {
	ID        int32  `json:"id"`
	MonsterID int32  `json:"monster_id"`
	Kind      string `json:"kind"`
	ItemID    int32  `json:"item_id"`
	Count     int32  `json:"count"`
}

// LoadKillRewardsFromJSONFile 读取击杀奖励 JSON 数组。
func LoadKillRewardsFromJSONFile(path string) ([]schema.CfgKillReward, error) {
	var rows []killRewardJSON
	if err := readJSONArray(path, &rows); err != nil {
		return nil, err
	}
	out := make([]schema.CfgKillReward, 0, len(rows))
	for _, r := range rows {
		if r.ID < 1 || r.MonsterID < 1 || r.ItemID < 1 || r.Count < 1 || r.Kind == "" {
			return nil, fmt.Errorf("invalid kill reward row in %s", path)
		}
		out = append(out, schema.CfgKillReward{
			ID: r.ID, MonsterID: r.MonsterID, Kind: r.Kind, ItemID: r.ItemID, Count: r.Count,
		})
	}
	return out, nil
}
