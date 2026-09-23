package importdata

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/example/mmo-server/gameconfig/pkg/schema"
)

const (
	// SceneTableFile 是地图表 JSON 文件名（与 Luban 导出命名一致）。
	SceneTableFile = "scene_tbscene.json"
)

type sceneJSON struct {
	ID          int32   `json:"id"`
	Name        string  `json:"name"`
	Main        bool    `json:"main"`
	AllowCombat bool    `json:"allow_combat"`
	MaxOnline   int32   `json:"max_online"`
	MaxLines    int32   `json:"max_lines"`
	SpawnX      float32 `json:"spawn_x"`
	SpawnY      float32 `json:"spawn_y"`
	SpawnZ      float32 `json:"spawn_z"`
	SwitchCdMs  int32   `json:"switch_cd_ms"`
}

// LoadScenesFromJSONFile 读取地图 JSON。全表必须正好有一张主城。
func LoadScenesFromJSONFile(path string) ([]schema.CfgScene, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var rows []sceneJSON
	if err := json.Unmarshal(b, &rows); err != nil {
		return nil, fmt.Errorf("parse json %s: %w", path, err)
	}
	out := make([]schema.CfgScene, 0, len(rows))
	mains := 0
	seen := map[int32]struct{}{}
	for _, r := range rows {
		if r.ID < 1 {
			return nil, fmt.Errorf("invalid scene row in %s", path)
		}
		if _, ok := seen[r.ID]; ok {
			return nil, fmt.Errorf("duplicate scene id %d in %s", r.ID, path)
		}
		seen[r.ID] = struct{}{}
		if r.Main {
			mains++
		}
		out = append(out, schema.CfgScene{
			ID: r.ID, Name: r.Name, Main: r.Main, AllowCombat: r.AllowCombat,
			MaxOnline: r.MaxOnline, MaxLines: r.MaxLines,
			SpawnX: r.SpawnX, SpawnY: r.SpawnY, SpawnZ: r.SpawnZ, SwitchCdMs: r.SwitchCdMs,
		})
	}
	if mains != 1 {
		return nil, fmt.Errorf("scene table must contain exactly one main row, got %d", mains)
	}
	return out, nil
}
