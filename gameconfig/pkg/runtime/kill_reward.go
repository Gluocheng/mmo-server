package runtime

import (
	"strconv"

	"github.com/example/mmo-server/gameconfig/pkg/schema"
)

// KillRewardDef 是一只怪物一种击杀奖励。
type KillRewardDef struct {
	ID        int32
	MonsterID int32
	Kind      string
	ItemID    int32
	Count     int32
}

// KillReward 按怪物和种类读取奖励。没有这一行时 ok 为 false。
func KillReward(monsterID int32, kind string) (KillRewardDef, bool) {
	s := getSnapshot()
	if s == nil || s.tables == nil {
		return KillRewardDef{}, false
	}
	def, ok := s.tables.killRewards[killRewardKey(monsterID, kind)]
	return def, ok
}

// BuildKillRewards 用内存行替换击杀奖励，保留其它表。仅测试或启动前灌入。
func BuildKillRewards(rows []KillRewardDef) {
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
	m := make(map[string]KillRewardDef, len(rows))
	for _, row := range rows {
		if row.MonsterID < 1 || row.ItemID < 1 || row.Count < 1 || row.Kind == "" {
			continue
		}
		m[killRewardKey(row.MonsterID, row.Kind)] = row
	}
	base.killRewards = m
	swapSnapshot(&snapshot{version: ver, tableCount: tc, tables: base})
}

// killRewardKey 用怪物模板和种类拼内存索引。同一次击杀里种类不会重复。
func killRewardKey(monsterID int32, kind string) string {
	return strconv.FormatInt(int64(monsterID), 10) + ":" + kind
}

// killRewardsFromSchema 把配表行收成按怪物和种类查找的表。数量或种类为空的行丢弃。
func killRewardsFromSchema(rows []schema.CfgKillReward) map[string]KillRewardDef {
	m := make(map[string]KillRewardDef, len(rows))
	for _, r := range rows {
		if r.MonsterID < 1 || r.Count < 1 || r.Kind == "" {
			continue
		}
		m[killRewardKey(r.MonsterID, r.Kind)] = KillRewardDef{
			ID: r.ID, MonsterID: r.MonsterID, Kind: r.Kind, ItemID: r.ItemID, Count: r.Count,
		}
	}
	return m
}
