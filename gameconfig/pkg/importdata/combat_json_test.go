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
	if len(skills) != 2 || skills[1].ID != 2 || skills[1].Damage != 8 || skills[1].BuffID != 1 {
		t.Fatalf("skills %+v", skills)
	}
	buffs, err := LoadBuffsFromJSONFile(filepath.Join(dir, BuffTableFile))
	if err != nil {
		t.Fatal(err)
	}
	if len(buffs) != 1 || buffs[0].Effect != "dot" || buffs[0].Value != 4 {
		t.Fatalf("buffs %+v", buffs)
	}
	row, err := LoadCombatConstFromJSONFile(filepath.Join(dir, CombatConstTableFile))
	if err != nil {
		t.Fatal(err)
	}
	if row.ID != 1 || row.MaxHP != 100 || row.TickMs != 100 || row.FrameEventCap != 64 {
		t.Fatalf("const %+v", row)
	}
}
