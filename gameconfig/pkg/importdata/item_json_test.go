package importdata_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/example/mmo-server/gameconfig/pkg/importdata"
)

func TestLoadItemsFromJSONFile(t *testing.T) {
	path := filepath.Join("..", "..", "gen", "data", importdata.ItemTableFile)
	if _, err := os.Stat(path); err != nil {
		t.Skip("gen data not present")
	}
	table, _, err := importdata.LoadItemsFromJSONFile(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	rows := importdata.ItemsToSchema(table.GetDataList())
	if len(rows) < 4 {
		t.Fatalf("expected demo items, got %d", len(rows))
	}
	for _, row := range rows {
		if row.ID != 1001 {
			continue
		}
		if row.UseBuffID != 2 {
			t.Fatalf("potion use_buff_id=%d", row.UseBuffID)
		}
		return
	}
	t.Fatal("item 1001 missing")
}
