package persistence

import (
	"context"
	"fmt"
	"log"

	"github.com/example/mmo-server/internal/protocol"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
)

var playerCacheJSON = protojson.MarshalOptions{EmitUnpopulated: true}

func playerUIDKey(uid int64) string {
	return fmt.Sprintf("%s:player:uid:%d", KeyPrefix(), uid)
}

func playerInfoFromModel(model Player) *protocol.PlayerInfo {
	return &protocol.PlayerInfo{
		PlayerId: model.PlayerID,
		Name:     model.Name,
	}
}

func schedulePlayerCacheRefresh(ctx context.Context, uid int64, player *protocol.PlayerInfo) {
	if rdb == nil {
		return
	}
	b, err := playerCacheJSON.Marshal(player)
	if err != nil {
		log.Printf("persistence: marshal player cache failed uid=%d err=%v", uid, err)
		return
	}
	cacheKey := playerUIDKey(uid)
	AfterCommit(ctx, func(commitCtx context.Context) {
		if err := rdb.Set(commitCtx, cacheKey, b, CacheTTL()).Err(); err != nil {
			log.Printf("persistence: after commit player cache failed uid=%d err=%v", uid, err)
		}
	})
}

func writePlayerCache(ctx context.Context, uid int64, player *protocol.PlayerInfo) {
	if rdb == nil {
		return
	}
	b, err := playerCacheJSON.Marshal(player)
	if err != nil {
		return
	}
	_ = rdb.Set(ctx, playerUIDKey(uid), b, CacheTTL()).Err()
}

func getPlayerByUIDFromDB(ctx context.Context, uid int64) (*protocol.PlayerInfo, bool, error) {
	var model Player
	err := DBFromContext(ctx).WithContext(ctx).Where("uid = ?", uid).First(&model).Error
	if err == gorm.ErrRecordNotFound {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return playerInfoFromModel(model), true, nil
}

func GetPlayerByUIDContext(parent context.Context, uid int64) (*protocol.PlayerInfo, bool, error) {
	if err := ensureDB(); err != nil {
		return nil, false, err
	}
	if uid < 1 {
		return nil, false, nil
	}

	ctx, cancel := opContext(parent)
	defer cancel()

	if rdb != nil {
		cacheKey := playerUIDKey(uid)
		if raw, err := rdb.Get(ctx, cacheKey).Bytes(); err == nil {
			var player protocol.PlayerInfo
			if jsonErr := protojson.Unmarshal(raw, &player); jsonErr == nil && player.GetPlayerId() > 0 {
				return proto.Clone(&player).(*protocol.PlayerInfo), true, nil
			}
		}
	}

	player, found, err := getPlayerByUIDFromDB(ctx, uid)
	if err != nil || !found {
		return player, found, err
	}
	writePlayerCache(ctx, uid, player)
	return proto.Clone(player).(*protocol.PlayerInfo), true, nil
}

func GetPlayerByUID(uid int64) (*protocol.PlayerInfo, bool, error) {
	return GetPlayerByUIDContext(context.Background(), uid)
}

func createPlayerInTx(ctx context.Context, uid int64, name string) (*protocol.PlayerInfo, bool, error) {
	if old, found, err := getPlayerByUIDFromDB(ctx, uid); err != nil {
		return nil, false, err
	} else if found {
		return proto.Clone(old).(*protocol.PlayerInfo), false, nil
	}

	model := Player{
		UID:  uid,
		Name: name,
	}
	if err := DBFromContext(ctx).WithContext(ctx).Create(&model).Error; err != nil {
		var existed Player
		qErr := DBFromContext(ctx).WithContext(ctx).Where("uid = ?", uid).First(&existed).Error
		if qErr != nil {
			return nil, false, err
		}
		return playerInfoFromModel(existed), false, nil
	}

	player := playerInfoFromModel(model)
	schedulePlayerCacheRefresh(ctx, uid, player)
	return player, true, nil
}

func CreatePlayerContext(parent context.Context, uid int64, name string) (*protocol.PlayerInfo, bool, error) {
	return CreatePlayerForUIDContext(parent, uid, name)
}

// CreatePlayer 兼容入口，委托给 CreatePlayerForUID。
func CreatePlayer(uid int64, name string) (*protocol.PlayerInfo, bool, error) {
	return CreatePlayerContext(context.Background(), uid, name)
}
