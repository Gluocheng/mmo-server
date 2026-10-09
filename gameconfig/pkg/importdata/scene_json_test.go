package importdata

import (
	"path/filepath"
	"testing"
)

func TestLoadSceneSeedJSON(t *testing.T) {
	rows, err := LoadScenesFromJSONFile(filepath.Join("..", "..", "gen", "data", SceneTableFile))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 4 || !rows[0].Main || rows[0].AllowCombat || rows[0].MaxLines != 5 {
		t.Fatalf("scenes %+v", rows)
	}
	if rows[1].SwitchCdMs != 10000 || rows[1].MaxLines != 1 || !rows[1].AllowCombat {
		t.Fatalf("arena %+v", rows[1])
	}
	if rows[2].SpawnX != 10 || rows[2].SpawnZ != 10 {
		t.Fatalf("wild %+v", rows[2])
	}
	if rows[3].Name != "世界BOSS" || !rows[3].AllowCombat || rows[3].MaxOnline != 30 || rows[3].MaxLines != 3 || rows[3].SwitchCdMs != 10000 {
		t.Fatalf("world boss %+v", rows[3])
	}
}
