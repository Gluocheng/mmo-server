package runtime

import "github.com/example/mmo-server/gameconfig/pkg/schema"

// SceneDef 地图配置视图。MaxLines 小于 2 表示这张图不能分线。
type SceneDef struct {
	ID          int32
	Name        string
	Main        bool
	AllowCombat bool
	MaxOnline   int32
	MaxLines    int32
	SpawnX      float32
	SpawnY      float32
	SpawnZ      float32
	SwitchCdMs  int32
}

// LineCount 返回这张图实际可用的线数。不能分线时为 1。
func (s SceneDef) LineCount() int32 {
	if s.MaxLines >= 2 {
		return s.MaxLines
	}
	return 1
}

// Scene 按 id 读取地图。
func Scene(id int32) (SceneDef, bool) {
	s := getSnapshot()
	if s == nil || s.tables == nil || s.tables.scenes == nil {
		return SceneDef{}, false
	}
	def, ok := s.tables.scenes[id]
	return def, ok
}

// MainScene 返回唯一的登录出生图。没有或多于一张时 ok 为 false。
func MainScene() (SceneDef, bool) {
	rows := Scenes()
	var found SceneDef
	n := 0
	for _, row := range rows {
		if !row.Main {
			continue
		}
		found = row
		n++
	}
	if n != 1 {
		return SceneDef{}, false
	}
	return found, true
}

// Scenes 按 id 升序返回全部地图。
func Scenes() []SceneDef {
	s := getSnapshot()
	if s == nil || s.tables == nil || len(s.tables.scenes) == 0 {
		return nil
	}
	out := make([]SceneDef, 0, len(s.tables.scenes))
	for _, row := range s.tables.scenes {
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

// SceneAllowsCombat 当前配表是否允许在这张图放技能。地图不存在时为 false。
func SceneAllowsCombat(id int32) bool {
	def, ok := Scene(id)
	return ok && def.AllowCombat
}

// BuildScenes 用内存行替换地图表，保留已有的其它配置。仅测试或启动前灌入。
func BuildScenes(rows []SceneDef) {
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
	m := make(map[int32]SceneDef, len(rows))
	for _, row := range rows {
		m[row.ID] = row
	}
	base.scenes = m
	swapSnapshot(&snapshot{version: ver, tableCount: tc, tables: base})
}

func scenesFromSchema(rows []schema.CfgScene) map[int32]SceneDef {
	m := make(map[int32]SceneDef, len(rows))
	for _, r := range rows {
		m[r.ID] = SceneDef{
			ID: r.ID, Name: r.Name, Main: r.Main, AllowCombat: r.AllowCombat,
			MaxOnline: r.MaxOnline, MaxLines: r.MaxLines,
			SpawnX: r.SpawnX, SpawnY: r.SpawnY, SpawnZ: r.SpawnZ, SwitchCdMs: r.SwitchCdMs,
		}
	}
	return m
}
