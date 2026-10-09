package reward

import (
	"context"
	"testing"

	gcruntime "github.com/example/mmo-server/gameconfig/pkg/runtime"
	"github.com/example/mmo-server/internal/gameapp/world"
	"github.com/example/mmo-server/internal/persistence"
)

func TestApplySoloStacksRankAndLastHit(t *testing.T) {
	gdb := persistence.UseMemoryDBForTest(t)
	if err := gcruntime.SeedDemoItems(context.Background(), gdb); err != nil {
		t.Fatal(err)
	}
	gcruntime.BuildKillRewards([]gcruntime.KillRewardDef{
		{MonsterID: 2, Kind: "solo", ItemID: 1002, Count: 20},
		{MonsterID: 2, Kind: "rank1", ItemID: 1002, Count: 30},
		{MonsterID: 2, Kind: "last", ItemID: 1001, Count: 5},
	})
	world.Place(1, "gate.user", 4, 1, 0, 0, 0)
	t.Cleanup(func() { world.Leave(1) })

	Apply(nil, Kill{
		MonsterUID: 900, TemplateID: 2, SceneID: 4, Line: 1, LastHit: 1,
		Hits: []Hurt{{UID: 1, PlayerID: 100, Damage: 10, At: 1}},
	})
	coin, err := persistence.GetBagByPlayerID(100, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(coin.Items) != 1 || coin.Items[0].ItemId != 1002 || coin.Items[0].Count != 50 {
		t.Fatalf("coins %+v", coin.Items)
	}
	potion, err := persistence.GetBagByPlayerID(100, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(potion.Items) != 1 || potion.Items[0].ItemId != 1001 || potion.Items[0].Count != 5 {
		t.Fatalf("potions %+v", potion.Items)
	}
}
