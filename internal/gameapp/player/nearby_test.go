package player

import (
	"testing"

	"github.com/example/mmo-server/internal/gameapp/world"
)

func TestNearbyProtoOmitsMonsterFromPlayers(t *testing.T) {
	nearby, ids := nearbyProto([]world.Nearby{
		{UID: 11, ActorType: world.ActorPlayer},
		{UID: 9001, ActorType: world.ActorMonster, ConfigID: 1},
	})
	if len(nearby) != 2 || nearby[1].Uid != 9001 || nearby[1].ActorType != world.ActorMonster || nearby[1].ConfigId != 1 {
		t.Fatalf("nearby %+v", nearby)
	}
	if len(ids) != 1 || ids[0] != 11 {
		t.Fatalf("players %v", ids)
	}
}
