package persistence

import (
	"context"
	"errors"
	"fmt"
	"sort"

	clog "github.com/cherry-game/cherry/logger"
	gcruntime "github.com/example/mmo-server/gameconfig/pkg/runtime"
	"github.com/example/mmo-server/internal/protocol"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// 背包容量与堆叠上限（与协议、错误码 40021/40022 一致）。
const (
	MaxBagStack int32 = 9999 // 单槽最大堆叠数量
	MaxBagSlots int32 = 32   // 固定槽位数，slot 取值 0..31
)

var (
	ErrBagNotEnough   = errors.New("bag item not enough") // 扣除数量超过持有
	ErrBagInvalid     = errors.New("bag item invalid")    // itemId/count 非法或 move 合并失败
	ErrBagSlotInvalid = errors.New("bag slot invalid")    // 槽位越界或源槽为空
	ErrBagFull        = errors.New("bag full")            // 无空槽可放入
	ErrItemNotFound   = errors.New("item not found in config")
)

var bagCacheJSON = protojson.MarshalOptions{EmitUnpopulated: true}

func bagPlayerKey(playerID int64) string {
	return fmt.Sprintf("%s:bag:player:%d", KeyPrefix(), playerID)
}

func validateSlot(slot int32) error {
	if slot < 0 || slot >= MaxBagSlots {
		return ErrBagSlotInvalid
	}
	return nil
}

func validateBagItem(itemID, count int32) error {
	if itemID < 1 || count < 1 {
		return ErrBagInvalid
	}
	if err := gcruntime.ValidateItemID(itemID); err != nil {
		return ErrItemNotFound
	}
	max := effectiveMaxStack(itemID)
	if max < 1 || count > max {
		return ErrBagInvalid
	}
	return nil
}

// effectiveMaxStack 取 min(全局硬顶, 配置 max_stack)。
func effectiveMaxStack(itemID int32) int32 {
	ms := gcruntime.MaxStack(itemID)
	if ms < 1 {
		return 0
	}
	if ms > MaxBagStack {
		return MaxBagStack
	}
	return ms
}

func bagListFromModels(models []InventoryItem) *protocol.BagListResponse {
	sort.Slice(models, func(i, j int) bool {
		return models[i].Slot < models[j].Slot
	})
	rsp := &protocol.BagListResponse{}
	for _, m := range models {
		if m.Count < 1 || m.ItemID < 1 {
			continue
		}
		rsp.Items = append(rsp.Items, &protocol.BagItem{
			Slot:   m.Slot,
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

// scheduleBagCacheRefresh 在事务提交后从 MySQL 重载背包并写入 Redis（AfterCommit）。
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

// GetBagByPlayerIDContext 读取背包：优先 Redis protojson，未命中则查库并回填缓存。
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

// GetBagByPlayerID 读取背包（Background 上下文）。
func GetBagByPlayerID(playerID int64) (*protocol.BagListResponse, error) {
	return GetBagByPlayerIDContext(context.Background(), playerID)
}

func loadItemAtSlotForUpdate(txDB *gorm.DB, ctx context.Context, playerID int64, slot int32) (*InventoryItem, error) {
	if err := validateSlot(slot); err != nil {
		return nil, err
	}
	var item InventoryItem
	err := txDB.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("player_id = ? AND slot = ?", playerID, slot).
		First(&item).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func findEmptySlotInTx(ctx context.Context, playerID int64) (int32, error) {
	txDB := DBFromContext(ctx).WithContext(ctx)
	var used []int32
	if err := txDB.Model(&InventoryItem{}).
		Where("player_id = ?", playerID).
		Pluck("slot", &used).Error; err != nil {
		return -1, err
	}
	occupied := make(map[int32]struct{}, len(used))
	for _, s := range used {
		occupied[s] = struct{}{}
	}
	for slot := int32(0); slot < MaxBagSlots; slot++ {
		if _, ok := occupied[slot]; !ok {
			return slot, nil
		}
	}
	return -1, ErrBagFull
}

// addOrStackItemInTx 先向同 item_id 的已有槽堆叠，剩余数量占最小号空槽（可跨多槽）。
func addOrStackItemInTx(ctx context.Context, playerID int64, itemID, count int32) error {
	if err := validateBagItem(itemID, count); err != nil {
		return err
	}
	txDB := DBFromContext(ctx).WithContext(ctx)

	var stacks []InventoryItem
	if err := txDB.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("player_id = ? AND item_id = ?", playerID, itemID).
		Find(&stacks).Error; err != nil {
		return err
	}
	remaining := count
	for i := range stacks {
		if remaining < 1 {
			break
		}
		room := effectiveMaxStack(itemID) - stacks[i].Count
		if room < 1 {
			continue
		}
		add := remaining
		if add > room {
			add = room
		}
		if err := txDB.Model(&stacks[i]).Update("count", stacks[i].Count+add).Error; err != nil {
			return err
		}
		remaining -= add
	}
	for remaining > 0 {
		slot, err := findEmptySlotInTx(ctx, playerID)
		if err != nil {
			return err
		}
		put := remaining
		maxStack := effectiveMaxStack(itemID)
		if put > maxStack {
			put = maxStack
		}
		if err := txDB.Create(&InventoryItem{
			PlayerID: playerID,
			Slot:     slot,
			ItemID:   itemID,
			Count:    put,
		}).Error; err != nil {
			return err
		}
		remaining -= put
	}
	return nil
}

// AddOrStackItemContext 发放物品：WithinTx 写库，提交后刷新 Redis 背包缓存。
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

// AddOrStackItem 发放物品（Background 上下文）。
func AddOrStackItem(playerID int64, itemID, count int32) error {
	return AddOrStackItemContext(context.Background(), playerID, itemID, count)
}

// removeFromSlotInTx 从指定槽位扣除 count；扣完则删行，否则减数量。
func removeFromSlotInTx(ctx context.Context, playerID int64, slot, count int32) error {
	if err := validateSlot(slot); err != nil {
		return err
	}
	if count < 1 || count > MaxBagStack {
		return ErrBagInvalid
	}
	txDB := DBFromContext(ctx).WithContext(ctx)
	item, err := loadItemAtSlotForUpdate(txDB, ctx, playerID, slot)
	if err != nil {
		return err
	}
	if item == nil || item.Count < count {
		return ErrBagNotEnough
	}
	if item.Count == count {
		return txDB.Delete(item).Error
	}
	return txDB.Model(item).Update("count", item.Count-count).Error
}

// removeByItemIDInTx 按 item_id 从 slot 升序各槽合计扣除 count（可跨多槽）。
func removeByItemIDInTx(ctx context.Context, playerID int64, itemID, count int32) error {
	if err := validateBagItem(itemID, count); err != nil {
		return err
	}
	txDB := DBFromContext(ctx).WithContext(ctx)
	var stacks []InventoryItem
	if err := txDB.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("player_id = ? AND item_id = ?", playerID, itemID).
		Order("slot asc").
		Find(&stacks).Error; err != nil {
		return err
	}
	remaining := count
	for i := range stacks {
		if remaining < 1 {
			break
		}
		take := stacks[i].Count
		if take > remaining {
			take = remaining
		}
		if stacks[i].Count == take {
			if err := txDB.Delete(&stacks[i]).Error; err != nil {
				return err
			}
		} else {
			if err := txDB.Model(&stacks[i]).Update("count", stacks[i].Count-take).Error; err != nil {
				return err
			}
		}
		remaining -= take
	}
	if remaining > 0 {
		return ErrBagNotEnough
	}
	return nil
}

// RemoveItemContext 扣除物品：bySlot 走单槽，否则按 itemId 跨槽扣；提交后刷缓存。
func RemoveItemContext(parent context.Context, playerID int64, slot int32, bySlot bool, itemID, count int32) error {
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
		if bySlot {
			return removeFromSlotInTx(txCtx, playerID, slot, count)
		}
		return removeByItemIDInTx(txCtx, playerID, itemID, count)
	})
	if err != nil {
		return err
	}
	scheduleBagCacheRefresh(ctx, playerID)
	return nil
}

// RemoveItem 按 itemId 跨槽扣除（协议 bySlot=false）。
func RemoveItem(playerID int64, itemID, count int32) error {
	return RemoveItemContext(context.Background(), playerID, 0, false, itemID, count)
}

// RemoveItemAtSlot 从指定槽位扣除（协议 bySlot=true）。
func RemoveItemAtSlot(playerID int64, slot, count int32) error {
	return RemoveItemContext(context.Background(), playerID, slot, true, 0, count)
}

// moveItemInTx 移动/合并/交换：目标空槽则改 slot；同 item 则合并（源槽可剩）；异 item 则交换两栈。
func moveItemInTx(ctx context.Context, playerID int64, fromSlot, toSlot int32) error {
	if err := validateSlot(fromSlot); err != nil {
		return err
	}
	if err := validateSlot(toSlot); err != nil {
		return err
	}
	if fromSlot == toSlot {
		return nil
	}
	txDB := DBFromContext(ctx).WithContext(ctx)
	from, err := loadItemAtSlotForUpdate(txDB, ctx, playerID, fromSlot)
	if err != nil {
		return err
	}
	if from == nil || from.ItemID < 1 || from.Count < 1 {
		return ErrBagSlotInvalid
	}
	to, err := loadItemAtSlotForUpdate(txDB, ctx, playerID, toSlot)
	if err != nil {
		return err
	}
	if to == nil {
		return txDB.Model(from).Update("slot", toSlot).Error
	}
	if to.ItemID == from.ItemID {
		maxStack := effectiveMaxStack(from.ItemID)
		room := maxStack - to.Count
		if room < 1 {
			return ErrBagInvalid
		}
		move := from.Count
		if move > room {
			move = room
		}
		if err := txDB.Model(to).Update("count", to.Count+move).Error; err != nil {
			return err
		}
		if from.Count == move {
			return txDB.Delete(from).Error
		}
		return txDB.Model(from).Update("count", from.Count-move).Error
	}
	fromItemID, fromCount := from.ItemID, from.Count
	toItemID, toCount := to.ItemID, to.Count
	if err := txDB.Model(from).Updates(map[string]interface{}{
		"item_id": toItemID,
		"count":   toCount,
	}).Error; err != nil {
		return err
	}
	return txDB.Model(to).Updates(map[string]interface{}{
		"item_id": fromItemID,
		"count":   fromCount,
	}).Error
}

// MoveItemContext 槽位移动：WithinTx 内 moveItemInTx，提交后刷缓存。
func MoveItemContext(parent context.Context, playerID int64, fromSlot, toSlot int32) error {
	if err := ensureDB(); err != nil {
		return err
	}
	if playerID < 1 {
		return ErrBagInvalid
	}
	ctx, cancel := opContext(parent)
	defer cancel()
	err := WithinTx(ctx, func(txCtx context.Context) error {
		return moveItemInTx(txCtx, playerID, fromSlot, toSlot)
	})
	if err != nil {
		return err
	}
	scheduleBagCacheRefresh(ctx, playerID)
	return nil
}

// MoveItem 槽位移动（Background 上下文）。
func MoveItem(playerID int64, fromSlot, toSlot int32) error {
	return MoveItemContext(context.Background(), playerID, fromSlot, toSlot)
}

// splitItemInTx 从源槽拆出 count 到新空槽；要求源槽数量严格大于 count。
func splitItemInTx(ctx context.Context, playerID int64, fromSlot, count int32) error {
	if err := validateSlot(fromSlot); err != nil {
		return err
	}
	if count < 1 || count > MaxBagStack {
		return ErrBagInvalid
	}
	txDB := DBFromContext(ctx).WithContext(ctx)
	from, err := loadItemAtSlotForUpdate(txDB, ctx, playerID, fromSlot)
	if err != nil {
		return err
	}
	if from == nil || from.Count <= count {
		return ErrBagNotEnough
	}
	emptySlot, err := findEmptySlotInTx(ctx, playerID)
	if err != nil {
		return err
	}
	if err := txDB.Create(&InventoryItem{
		PlayerID: playerID,
		Slot:     emptySlot,
		ItemID:   from.ItemID,
		Count:    count,
	}).Error; err != nil {
		return err
	}
	if from.Count == count {
		return txDB.Delete(from).Error
	}
	return txDB.Model(from).Update("count", from.Count-count).Error
}

// SplitItemContext 拆分堆叠：WithinTx 写库，提交后刷缓存。
func SplitItemContext(parent context.Context, playerID int64, fromSlot, count int32) error {
	if err := ensureDB(); err != nil {
		return err
	}
	if playerID < 1 {
		return ErrBagInvalid
	}
	ctx, cancel := opContext(parent)
	defer cancel()
	err := WithinTx(ctx, func(txCtx context.Context) error {
		return splitItemInTx(txCtx, playerID, fromSlot, count)
	})
	if err != nil {
		return err
	}
	scheduleBagCacheRefresh(ctx, playerID)
	return nil
}

// SplitItem 拆分堆叠（Background 上下文）。
func SplitItem(playerID int64, fromSlot, count int32) error {
	return SplitItemContext(context.Background(), playerID, fromSlot, count)
}
