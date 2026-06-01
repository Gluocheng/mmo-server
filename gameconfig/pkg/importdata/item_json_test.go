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
	items, err := importdata.LoadItemsFromJSONFile(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(items) < 4 {
		t.Fatalf("expected demo items, got %d", len(items))
	}
}
