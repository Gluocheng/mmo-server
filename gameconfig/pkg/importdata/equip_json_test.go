package importdata

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEquipAttrRejectsUnsettledStat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "equip_attr.json")
	body := []byte(`[{"id":1,"item_id":2001,"stat":"dodge","mode":"percent","value":5}]`)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	rules := map[string]StatRule{
		"dodge":     {AllowPercent: true},
		"crit_rate": {AllowPercent: true, Settle: true},
	}
	if _, err := LoadEquipAttrsFromJSONFile(path, rules); err == nil {
		t.Fatal("dodge should be rejected")
	}
	body = []byte(`[{"id":1,"item_id":2001,"stat":"crit_rate","mode":"percent","value":10}]`)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	rows, err := LoadEquipAttrsFromJSONFile(path, rules)
	if err != nil || len(rows) != 1 || rows[0].Value != 10 {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
}
