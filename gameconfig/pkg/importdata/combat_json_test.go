package importdata

import (
	"path/filepath"
	"testing"
)

func TestLoadCombatSeedJSON(t *testing.T) {
	dir := filepath.Join("..", "..", "gen", "data")
	skills, err := LoadSkillsFromJSONFile(filepath.Join(dir, SkillTableFile))
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 3 || skills[1].ID != 2 || skills[1].Damage != 0 || skills[1].Factor != 80 || skills[1].BuffID != 1 || skills[2].BuffID != 3 {
		t.Fatalf("skills %+v", skills)
	}
	buffs, err := LoadBuffsFromJSONFile(filepath.Join(dir, BuffTableFile))
	if err != nil {
		t.Fatal(err)
	}
	if len(buffs) != 4 || buffs[0].Effect != "dot" || buffs[0].Value != 4 || buffs[1].Effect != "hot" || buffs[2].Effect != "stun" || buffs[3].Effect != "attr" || buffs[3].Stat != "攻击" || buffs[3].Mode != "flat" || buffs[3].Value != 5 {
		t.Fatalf("buffs %+v", buffs)
	}
	row, err := LoadCombatConstFromJSONFile(filepath.Join(dir, CombatConstTableFile))
	if err != nil {
		t.Fatal(err)
	}
	if row.ID != 1 || row.MaxHP != 100 || row.Attack != 10 || row.Defense != 0 || row.TickMs != 100 || row.FrameEventCap != 64 {
		t.Fatalf("const %+v", row)
	}
	stats, err := LoadStatsFromJSONFile(filepath.Join(dir, StatTableFile))
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 33 || !stats[1].Settle || stats[1].Name != "攻击" || stats[3].Settle {
		t.Fatalf("stats %+v", stats[:4])
	}
}
