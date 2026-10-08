package importdata

import (
	"path/filepath"
	"testing"
)

func TestLoadMonsterAndSpawnJSON(t *testing.T) {
	dir := filepath.Join("..", "..", "gen", "data")
	monsters, err := LoadMonstersFromJSONFile(filepath.Join(dir, MonsterTableFile))
	if err != nil {
		t.Fatal(err)
	}
	if len(monsters) != 1 || monsters[0].Name != "野狼" || monsters[0].HP != 80 || monsters[0].Attack != 6 || monsters[0].Defense != 2 || monsters[0].AggroRange != 6 {
		t.Fatalf("monsters %+v", monsters)
	}
	spawns, err := LoadSpawnsFromJSONFile(filepath.Join(dir, SpawnTableFile))
	if err != nil {
		t.Fatal(err)
	}
	if len(spawns) != 1 || spawns[0].Feature != "wild" || spawns[0].SceneID != 3 || spawns[0].X != 18 || spawns[0].Count != 1 || spawns[0].RespawnMs != 8000 {
		t.Fatalf("spawns %+v", spawns)
	}
}
