package persistence

import (
	"context"

	"github.com/example/mmo-server/internal/persistence/model"
	"github.com/example/mmo-server/internal/protocol"
	"gorm.io/gorm/clause"
)

// BagSlotChange 是内存背包 flush 时的一格变更（upsert 用）。
type BagSlotChange struct {
	Slot   int32
	ItemID int32
	Count  int32
	Level  int32
}

// LoadBagFromDB 直接从 MySQL 读指定背包，不走 Redis 缓存（内存化加载用）。
func LoadBagFromDB(ctx context.Context, playerID int64, bagType int32) (*protocol.BagListResponse, error) {
	if err := ensureDB(); err != nil {
		return nil, err
	}
	if playerID < 1 {
		return &protocol.BagListResponse{}, nil
	}
	return loadBagFromDB(ctx, playerID, bagType)
}

// BagTypesOwned 返回该玩家拥有物品的背包类型列表（内存化进场预热用）。
func BagTypesOwned(ctx context.Context, playerID int64) ([]int32, error) {
	if err := ensureDB(); err != nil {
		return nil, err
	}
	if playerID < 1 {
		return nil, nil
	}
	var types []int32
	err := DBFromContext(ctx).WithContext(ctx).
		Model(&model.InventoryItem{}).
		Where("player_id = ?", playerID).
		Distinct("bag_type").
		Pluck("bag_type", &types).Error
	return types, err
}

// FlushBagDiff 把内存背包的变更集写回 MySQL：upserts 逐槽按唯一索引 (player_id,bag_type,slot)
// upsert（item_id/count 覆盖），removes 逐槽删除。整批在一个 WithinTx 内，提交后删除 Redis 缓存。
func FlushBagDiff(ctx context.Context, playerID int64, bagType int32, upserts []BagSlotChange, removes []int32) error {
	if err := ensureDB(); err != nil {
		return err
	}
	if playerID < 1 {
		return ErrBagInvalid
	}
	err := WithinTx(ctx, func(txCtx context.Context) error {
		txDB := DBFromContext(txCtx).WithContext(txCtx)
		for _, c := range upserts {
			if err := txDB.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "player_id"}, {Name: "bag_type"}, {Name: "slot"}},
				DoUpdates: clause.AssignmentColumns([]string{"item_id", "count", "enhance_level"}),
			}).Create(&model.InventoryItem{
				PlayerID:     playerID,
				BagType:      bagType,
				Slot:         c.Slot,
				ItemID:       c.ItemID,
				Count:        c.Count,
				EnhanceLevel: c.Level,
			}).Error; err != nil {
				return err
			}
		}
		if len(removes) > 0 {
			if err := txDB.Where("player_id = ? AND bag_type = ? AND slot IN ?", playerID, bagType, removes).
				Delete(&model.InventoryItem{}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	// 提交后删除 Redis 缓存，下次读走 MySQL 重载（不再双写）。
	invalidateBagCache(ctx, playerID, bagType)
	return nil
}

// invalidateBagCache 删除某玩家某背包的 Redis 缓存。
func invalidateBagCache(ctx context.Context, playerID int64, bagType int32) {
	if rdb == nil {
		return
	}
	_ = rdb.Del(ctx, bagPlayerKey(playerID, bagType)).Err()
}
