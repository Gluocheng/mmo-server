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
	if len(monsters) != 3 || monsters[0].Name != "野狼" || monsters[0].Kind != "mob" || monsters[0].HP != 80 {
		t.Fatalf("monsters %+v", monsters)
	}
	if monsters[1].Kind != "boss" || monsters[1].Name != "荒原霸主" || monsters[2].Kind != "mob" {
		t.Fatalf("kinds %+v %+v", monsters[1], monsters[2])
	}
	spawns, err := LoadSpawnsFromJSONFile(filepath.Join(dir, SpawnTableFile))
	if err != nil {
		t.Fatal(err)
	}
	if len(spawns) != 4 || spawns[0].Feature != "wild" || spawns[0].SceneID != 3 || spawns[0].Line != 1 || spawns[0].Count != 1 {
		t.Fatalf("spawns %+v", spawns)
	}
	if spawns[1].Line != 0 || spawns[1].SceneID != 4 || spawns[3].Count != 4 {
		t.Fatalf("world spawns %+v", spawns[1:])
	}
	rewards, err := LoadKillRewardsFromJSONFile(filepath.Join(dir, KillRewardTableFile))
	if err != nil {
		t.Fatal(err)
	}
	if len(rewards) != 12 || rewards[0].Kind != "solo" || rewards[0].ItemID != 1002 || rewards[0].Count != 20 {
		t.Fatalf("rewards %+v", rewards[0])
	}
}
