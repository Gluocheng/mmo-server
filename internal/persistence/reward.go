package persistence

import (
	"context"
	"errors"

	"github.com/example/mmo-server/internal/persistence/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrRewardClaimMissing 表示待领取不存在或不属于这个角色。
var ErrRewardClaimMissing = errors.New("reward claim missing")

// KillDelivery 是一次发奖的去向。Placed 为 false 时整份进了待领取。
type KillDelivery struct {
	Placed  bool
	BagType int32
	ClaimID int64
}

// DeliverKillItem 尝试把整份奖励放进背包。放不下则写入待领取，不拆开。
func DeliverKillItem(ctx context.Context, playerID int64, itemID, count, monsterID int32) (KillDelivery, error) {
	if playerID < 1 || itemID < 1 || count < 1 {
		return KillDelivery{}, ErrBagInvalid
	}
	var out KillDelivery
	err := WithinTx(ctx, func(ctx context.Context) error {
		bagType, room, err := itemRoom(ctx, playerID, itemID)
		if err != nil {
			return err
		}
		out.BagType = bagType
		if room < count {
			row := model.RewardClaim{
				PlayerID: playerID, ItemID: itemID, Count: count,
				Reason: "kill", MonsterID: monsterID,
			}
			if err := DBFromContext(ctx).WithContext(ctx).Create(&row).Error; err != nil {
				return err
			}
			out.ClaimID = row.ID
			out.Placed = false
			return nil
		}
		if err := addOrStackItemInTx(ctx, playerID, bagType, itemID, count); err != nil {
			return err
		}
		scheduleBagCacheRefresh(ctx, playerID, bagType)
		out.Placed = true
		return nil
	})
	if err != nil {
		return KillDelivery{}, err
	}
	return out, nil
}

// ListRewardClaims 返回这个角色尚未领取的奖励，按 id 升序。
func ListRewardClaims(ctx context.Context, playerID int64) ([]model.RewardClaim, error) {
	if err := ensureDB(); err != nil {
		return nil, err
	}
	if playerID < 1 {
		return nil, nil
	}
	var rows []model.RewardClaim
	err := DBFromContext(ctx).WithContext(ctx).
		Where("player_id = ?", playerID).
		Order("id asc").
		Find(&rows).Error
	return rows, err
}

// TakeRewardClaim 把一条待领取放进背包并删除。背包仍满时保留原行，返回 ErrBagFull。
func TakeRewardClaim(ctx context.Context, playerID, claimID int64) (int32, error) {
	if playerID < 1 || claimID < 1 {
		return 0, ErrRewardClaimMissing
	}
	var bagType int32
	err := WithinTx(ctx, func(ctx context.Context) error {
		var row model.RewardClaim
		err := DBFromContext(ctx).WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND player_id = ?", claimID, playerID).
			First(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrRewardClaimMissing
		}
		if err != nil {
			return err
		}
		bt, room, err := itemRoom(ctx, playerID, row.ItemID)
		if err != nil {
			return err
		}
		if room < row.Count {
			return ErrBagFull
		}
		if err := addOrStackItemInTx(ctx, playerID, bt, row.ItemID, row.Count); err != nil {
			return err
		}
		if err := DBFromContext(ctx).WithContext(ctx).Delete(&row).Error; err != nil {
			return err
		}
		scheduleBagCacheRefresh(ctx, playerID, bt)
		bagType = bt
		return nil
	})
	return bagType, err
}

// itemRoom 计算这个道具还能放进多少个。返回背包类型和剩余容量。
// 容量是已有堆叠的空位，加上空槽能放下的数量。调用方据此决定整份进背包还是整份进待领取。
func itemRoom(ctx context.Context, playerID int64, itemID int32) (int32, int32, error) {
	bagType, err := resolvedBagType(itemID)
	if err != nil {
		return 0, 0, err
	}
	limit := slotCountFor(bagType)
	if limit < 1 {
		return bagType, 0, nil
	}
	maxStack := effectiveMaxStack(itemID)
	if maxStack < 1 {
		maxStack = 1
	}
	txDB := DBFromContext(ctx).WithContext(ctx)
	var stacks []model.InventoryItem
	if err := txDB.Where("player_id = ? AND bag_type = ? AND item_id = ?", playerID, bagType, itemID).
		Find(&stacks).Error; err != nil {
		return 0, 0, err
	}
	var room int32
	for _, s := range stacks {
		if s.Count < maxStack {
			room += maxStack - s.Count
		}
	}
	var used int64
	if err := txDB.Model(&model.InventoryItem{}).
		Where("player_id = ? AND bag_type = ?", playerID, bagType).
		Count(&used).Error; err != nil {
		return 0, 0, err
	}
	empty := limit - int32(used)
	if empty > 0 {
		room += empty * maxStack
	}
	return bagType, room, nil
}
