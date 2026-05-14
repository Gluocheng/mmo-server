package persistence

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/example/mmo-server/internal/protocol"
	"gorm.io/gorm"
)

func playerUIDKey(uid int64) string {
	return fmt.Sprintf("%s:player:uid:%d", KeyPrefix(), uid)
}

func GetPlayerByUID(uid int64) (protocol.PlayerInfo, bool, error) {
	if err := Init(); err != nil {
		return protocol.PlayerInfo{}, false, err
	}
	if uid < 1 {
		return protocol.PlayerInfo{}, false, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cacheKey := playerUIDKey(uid)
	if raw, err := rdb.Get(ctx, cacheKey).Bytes(); err == nil {
		var player protocol.PlayerInfo
		if jsonErr := json.Unmarshal(raw, &player); jsonErr == nil && player.PlayerID > 0 {
			return player, true, nil
		}
	}

	var model Player
	err := db.WithContext(ctx).Where("uid = ?", uid).First(&model).Error
	if err == gorm.ErrRecordNotFound {
		return protocol.PlayerInfo{}, false, nil
	}
	if err != nil {
		return protocol.PlayerInfo{}, false, err
	}

	player := protocol.PlayerInfo{
		PlayerID: model.PlayerID,
		Name:     model.Name,
	}
	b, _ := json.Marshal(player)
	_ = rdb.Set(ctx, cacheKey, b, CacheTTL()).Err()
	return player, true, nil
}

func CreatePlayer(uid int64, name string) (protocol.PlayerInfo, bool, error) {
	if err := Init(); err != nil {
		return protocol.PlayerInfo{}, false, err
	}
	name = strings.TrimSpace(name)
	if uid < 1 || name == "" {
		return protocol.PlayerInfo{}, false, nil
	}

	if old, found, err := GetPlayerByUID(uid); err != nil {
		return protocol.PlayerInfo{}, false, err
	} else if found {
		return old, false, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	model := Player{
		UID:  uid,
		Name: name,
	}
	if err := db.WithContext(ctx).Create(&model).Error; err != nil {
		// 并发创建兜底
		var existed Player
		qErr := db.WithContext(ctx).Where("uid = ?", uid).First(&existed).Error
		if qErr != nil {
			return protocol.PlayerInfo{}, false, err
		}
		return protocol.PlayerInfo{
			PlayerID: existed.PlayerID,
			Name:     existed.Name,
		}, false, nil
	}

	player := protocol.PlayerInfo{
		PlayerID: model.PlayerID,
		Name:     model.Name,
	}
	b, _ := json.Marshal(player)
	_ = rdb.Set(ctx, playerUIDKey(uid), b, CacheTTL()).Err()
	return player, true, nil
}
