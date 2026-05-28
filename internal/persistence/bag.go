package persistence

import (
	"context"
	"errors"
	"fmt"

	clog "github.com/cherry-game/cherry/logger"
	"github.com/example/mmo-server/internal/protocol"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const MaxBagStack int32 = 9999

var (
	ErrBagNotEnough = errors.New("bag item not enough")
	ErrBagInvalid   = errors.New("bag item invalid")
)

var bagCacheJSON = protojson.MarshalOptions{EmitUnpopulated: true}

func bagPlayerKey(playerID int64) string {
	return fmt.Sprintf("%s:bag:player:%d", KeyPrefix(), playerID)
}

func validateBagItem(itemID, count int32) error {
	if itemID < 1 || count < 1 || count > MaxBagStack {
		return ErrBagInvalid
	}
	return nil
}

func bagListFromModels(models []InventoryItem) *protocol.BagListResponse {
	rsp := &protocol.BagListResponse{}
	for _, m := range models {
		if m.Count < 1 {
			continue
		}
		rsp.Items = append(rsp.Items, &protocol.BagItem{
			ItemId: m.ItemID,
			Count:  m.Count,
		})
	}
	return rsp
}

func loadBagFromDB(ctx context.Context, playerID int64) (*protocol.BagListResponse, error) {
	var models []InventoryItem
	err := DBFromContext(ctx).WithContext(ctx).
		Where("player_id = ?", playerID).
		Find(&models).Error
	if err != nil {
		return nil, err
	}
	return bagListFromModels(models), nil
}

func writeBagCache(ctx context.Context, playerID int64, bag *protocol.BagListResponse) {
	if rdb == nil || bag == nil {
		return
	}
	b, err := bagCacheJSON.Marshal(bag)
	if err != nil {
		return
	}
	_ = rdb.Set(ctx, bagPlayerKey(playerID), b, CacheTTL()).Err()
}

func scheduleBagCacheRefresh(ctx context.Context, playerID int64) {
	if rdb == nil {
		return
	}
	AfterCommit(ctx, func(commitCtx context.Context) {
		bag, err := loadBagFromDB(commitCtx, playerID)
		if err != nil {
			clog.Warnf("persistence: reload bag cache failed player_id=%d err=%v", playerID, err)
			return
		}
		b, err := bagCacheJSON.Marshal(bag)
		if err != nil {
			clog.Warnf("persistence: marshal bag cache failed player_id=%d err=%v", playerID, err)
			return
		}
		if err := rdb.Set(commitCtx, bagPlayerKey(playerID), b, CacheTTL()).Err(); err != nil {
			clog.Warnf("persistence: after commit bag cache failed player_id=%d err=%v", playerID, err)
		}
	})
}

func GetBagByPlayerIDContext(parent context.Context, playerID int64) (*protocol.BagListResponse, error) {
	if err := ensureDB(); err != nil {
		return nil, err
	}
	if playerID < 1 {
		return &protocol.BagListResponse{}, nil
	}

	ctx, cancel := opContext(parent)
	defer cancel()

	if rdb != nil {
		cacheKey := bagPlayerKey(playerID)
		if raw, err := rdb.Get(ctx, cacheKey).Bytes(); err == nil {
			var bag protocol.BagListResponse
			if jsonErr := protojson.Unmarshal(raw, &bag); jsonErr == nil {
				return proto.Clone(&bag).(*protocol.BagListResponse), nil
			}
		}
	}

	bag, err := loadBagFromDB(ctx, playerID)
	if err != nil {
		return nil, err
	}
	writeBagCache(ctx, playerID, bag)
	return proto.Clone(bag).(*protocol.BagListResponse), nil
}

func GetBagByPlayerID(playerID int64) (*protocol.BagListResponse, error) {
	return GetBagByPlayerIDContext(context.Background(), playerID)
}

func addOrStackItemInTx(ctx context.Context, playerID int64, itemID, count int32) error {
	if err := validateBagItem(itemID, count); err != nil {
		return err
	}

	txDB := DBFromContext(ctx).WithContext(ctx)
	var item InventoryItem
	err := txDB.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("player_id = ? AND item_id = ?", playerID, itemID).
		First(&item).Error
	if err == gorm.ErrRecordNotFound {
		return txDB.Create(&InventoryItem{
			PlayerID: playerID,
			ItemID:   itemID,
			Count:    count,
		}).Error
	}
	if err != nil {
		return err
	}

	newCount := item.Count + count
	if newCount > MaxBagStack {
		return ErrBagInvalid
	}
	return txDB.Model(&item).Update("count", newCount).Error
}

func AddOrStackItemContext(parent context.Context, playerID int64, itemID, count int32) error {
	if err := ensureDB(); err != nil {
		return err
	}
	if playerID < 1 {
		return ErrBagInvalid
	}
	if count < 1 {
		count = 1
	}

	ctx, cancel := opContext(parent)
	defer cancel()

	err := WithinTx(ctx, func(txCtx context.Context) error {
		return addOrStackItemInTx(txCtx, playerID, itemID, count)
	})
	if err != nil {
		return err
	}
	scheduleBagCacheRefresh(ctx, playerID)
	return nil
}

func AddOrStackItem(playerID int64, itemID, count int32) error {
	return AddOrStackItemContext(context.Background(), playerID, itemID, count)
}

func removeItemInTx(ctx context.Context, playerID int64, itemID, count int32) error {
	if err := validateBagItem(itemID, count); err != nil {
		return err
	}

	txDB := DBFromContext(ctx).WithContext(ctx)
	var item InventoryItem
	err := txDB.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("player_id = ? AND item_id = ?", playerID, itemID).
		First(&item).Error
	if err == gorm.ErrRecordNotFound {
		return ErrBagNotEnough
	}
	if err != nil {
		return err
	}
	if item.Count < count {
		return ErrBagNotEnough
	}
	if item.Count == count {
		return txDB.Delete(&item).Error
	}
	return txDB.Model(&item).Update("count", item.Count-count).Error
}

func RemoveItemContext(parent context.Context, playerID int64, itemID, count int32) error {
	if err := ensureDB(); err != nil {
		return err
	}
	if playerID < 1 {
		return ErrBagInvalid
	}
	if count < 1 {
		count = 1
	}

	ctx, cancel := opContext(parent)
	defer cancel()

	err := WithinTx(ctx, func(txCtx context.Context) error {
		return removeItemInTx(txCtx, playerID, itemID, count)
	})
	if err != nil {
		return err
	}
	scheduleBagCacheRefresh(ctx, playerID)
	return nil
}

func RemoveItem(playerID int64, itemID, count int32) error {
	return RemoveItemContext(context.Background(), playerID, itemID, count)
}
