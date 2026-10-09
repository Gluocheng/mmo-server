package runtime

import "github.com/example/mmo-server/gameconfig/pkg/schema"

// MonsterDef 怪物模板。实例生成时拷贝这些数值。
type MonsterDef struct {
	ID               int32
	Name             string
	Kind             string
	HP               int32
	Attack           int32
	Defense          int32
	MoveSpeed        float32
	AttackRange      float32
	AttackIntervalMs int32
	AggroRange       float32
	LeashRange       float32
}

// SpawnDef 刷怪点。Count 只怪沿 X 每隔 2 格一个出生点。
type SpawnDef struct {
	ID        int32
	Feature   string
	SceneID   int32
	Line      int32
	MonsterID int32
	X         float32
	Y         float32
	Z         float32
	Count     int32
	RespawnMs int32
}

// Monster 按模板 id 读取怪物。
func Monster(id int32) (MonsterDef, bool) {
	s := getSnapshot()
	if s == nil || s.tables == nil || s.tables.monsters == nil {
		return MonsterDef{}, false
	}
	def, ok := s.tables.monsters[id]
	return def, ok
}

// Monsters 按 id 升序返回全部怪物模板。
func Monsters() []MonsterDef {
	return orderedMonsters(monsterMap())
}

// Spawns 按 id 升序返回全部刷怪点。
func Spawns() []SpawnDef {
	s := getSnapshot()
	if s == nil || s.tables == nil || len(s.tables.spawns) == 0 {
		return nil
	}
	out := make([]SpawnDef, 0, len(s.tables.spawns))
	for _, row := range s.tables.spawns {
		out = append(out, row)
	}
	for i := 1; i < len(out); i++ {
		j := i
		for j > 0 && out[j].ID < out[j-1].ID {
			out[j], out[j-1] = out[j-1], out[j]
			j--
		}
	}
	return out
}

// BuildMonsters 用内存行替换怪物和刷怪点，保留其它表。仅测试或启动前灌入。
func BuildMonsters(monsters []MonsterDef, spawns []SpawnDef) {
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
	mm := make(map[int32]MonsterDef, len(monsters))
	for _, row := range monsters {
		mm[row.ID] = row
	}
	sm := make(map[int32]SpawnDef, len(spawns))
	for _, row := range spawns {
		sm[row.ID] = row
	}
	base.monsters = mm
	base.spawns = sm
	swapSnapshot(&snapshot{version: ver, tableCount: tc, tables: base})
}

func monsterMap() map[int32]MonsterDef {
	s := getSnapshot()
	if s == nil || s.tables == nil {
		return nil
	}
	return s.tables.monsters
}

func orderedMonsters(in map[int32]MonsterDef) []MonsterDef {
	if len(in) == 0 {
		return nil
	}
	out := make([]MonsterDef, 0, len(in))
	for _, row := range in {
		out = append(out, row)
	}
	for i := 1; i < len(out); i++ {
		j := i
		for j > 0 && out[j].ID < out[j-1].ID {
			out[j], out[j-1] = out[j-1], out[j]
			j--
		}
	}
	return out
}

func monstersFromSchema(rows []schema.CfgMonster) map[int32]MonsterDef {
	m := make(map[int32]MonsterDef, len(rows))
	for _, r := range rows {
		m[r.ID] = MonsterDef{
			ID: r.ID, Name: r.Name, Kind: r.Kind, HP: r.HP, Attack: r.Attack, Defense: r.Defense,
			MoveSpeed: r.MoveSpeed, AttackRange: r.AttackRange, AttackIntervalMs: r.AttackIntervalMs,
			AggroRange: r.AggroRange, LeashRange: r.LeashRange,
		}
	}
	return m
}

func spawnsFromSchema(rows []schema.CfgSpawn) map[int32]SpawnDef {
	m := make(map[int32]SpawnDef, len(rows))
	for _, r := range rows {
		m[r.ID] = SpawnDef{
			ID: r.ID, Feature: r.Feature, SceneID: r.SceneID, Line: r.Line, MonsterID: r.MonsterID,
			X: r.X, Y: r.Y, Z: r.Z, Count: r.Count, RespawnMs: r.RespawnMs,
		}
	}
	return m
}
