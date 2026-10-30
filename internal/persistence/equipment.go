package persistence

import (
	"context"

	"github.com/example/mmo-server/internal/persistence/model"
	"gorm.io/gorm/clause"
)

// EquipRow 是落库时的一个已穿部位。
type EquipRow struct {
	SlotID int32
	ItemID int32
	Level  int32
}

// LoadEquipment 读取角色当前穿着。没有行表示身上是空的。
func LoadEquipment(ctx context.Context, playerID int64) ([]EquipRow, error) {
	if err := ensureDB(); err != nil {
		return nil, err
	}
	if playerID < 1 {
		return nil, nil
	}
	var rows []model.PlayerEquipment
	err := DBFromContext(ctx).WithContext(ctx).
		Where("player_id = ?", playerID).
		Order("slot_id asc").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]EquipRow, 0, len(rows))
	for _, row := range rows {
		if row.ItemID < 1 {
			continue
		}
		out = append(out, EquipRow{SlotID: row.SlotID, ItemID: row.ItemID, Level: row.EnhanceLevel})
	}
	return out, nil
}

// FlushPlayer 把背包变更和当前穿着写进同一次事务。equipDirty 为真时整表替换该角色的穿着。
func FlushPlayer(ctx context.Context, playerID int64, diffs []BagDiff, worn []EquipRow, equipDirty bool) error {
	if err := ensureDB(); err != nil {
		return err
	}
	if playerID < 1 {
		return ErrBagInvalid
	}
	err := WithinTx(ctx, func(txCtx context.Context) error {
		txDB := DBFromContext(txCtx).WithContext(txCtx)
		for _, diff := range diffs {
			for _, c := range diff.Upserts {
				if err := txDB.Clauses(clause.OnConflict{
					Columns:   []clause.Column{{Name: "player_id"}, {Name: "bag_type"}, {Name: "slot"}},
					DoUpdates: clause.AssignmentColumns([]string{"item_id", "count", "enhance_level"}),
				}).Create(&model.InventoryItem{
					PlayerID:     playerID,
					BagType:      diff.BagType,
					Slot:         c.Slot,
					ItemID:       c.ItemID,
					Count:        c.Count,
					EnhanceLevel: c.Level,
				}).Error; err != nil {
					return err
				}
			}
			if len(diff.Removes) > 0 {
				if err := txDB.Where("player_id = ? AND bag_type = ? AND slot IN ?", playerID, diff.BagType, diff.Removes).
					Delete(&model.InventoryItem{}).Error; err != nil {
					return err
				}
			}
		}
		if !equipDirty {
			return nil
		}
		if err := txDB.Where("player_id = ?", playerID).Delete(&model.PlayerEquipment{}).Error; err != nil {
			return err
		}
		for _, row := range worn {
			if row.ItemID < 1 {
				continue
			}
			if err := txDB.Create(&model.PlayerEquipment{
				PlayerID: playerID, SlotID: row.SlotID, ItemID: row.ItemID, EnhanceLevel: row.Level,
			}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, diff := range diffs {
		invalidateBagCache(ctx, playerID, diff.BagType)
	}
	return nil
}

// BagDiff 是一个背包类型这一次要落库的变更。
type BagDiff struct {
	BagType int32
	Upserts []BagSlotChange
	Removes []int32
}
