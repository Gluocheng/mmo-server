package equip

import (
	"math"
	"math/rand/v2"

	gcruntime "github.com/example/mmo-server/gameconfig/pkg/runtime"
	"github.com/example/mmo-server/internal/code"
	"github.com/example/mmo-server/internal/protocol"
)

const (
	// 强化石和保护符。都在材料背包里。
	stoneItem   int32 = 1003
	protectItem int32 = 1004
	maxLevel    int32 = 10
)

// Material 是材料背包。Count 不够时不得开始掷点。
type Material interface {
	Count(itemID int32) int32
	Take(itemID, count int32) error
}

// Result 是一次已经掷点的强化结果。
type Result struct {
	Success     bool
	Dropped     bool
	ProtectUsed bool
	Level       int32
	Attrs       []*protocol.EquipAttr
}

// rollFn 按百分比掷点。percent 为 100 时一定成功，为 0 时一定失败。
var rollFn = defaultRoll

func defaultRoll(percent int32) bool {
	if percent >= 100 {
		return true
	}
	if percent <= 0 {
		return false
	}
	return int32(rand.IntN(100))+1 <= percent
}

func resetRoll() { rollFn = defaultRoll }

// SetRollsForTest 按顺序返回固定掷点，用完后再掷都失败。仅测试调用。
func SetRollsForTest(hits ...bool) {
	i := 0
	rollFn = func(int32) bool {
		if i >= len(hits) {
			return false
		}
		v := hits[i]
		i++
		return v
	}
}

// Enhance 强化指定穿着部位。材料不够或还没穿上时不掷点、不扣材料。
// 已经开始掷点后，强化石无论成败都扣。成功不消耗保护符。
func Enhance(playerID, uid int64, slotID int32, useProtect bool, mat Material) (Result, int32) {
	if playerID < 1 || mat == nil {
		return Result{}, code.PlayerNotEntered
	}
	if uid > 0 {
		uidByPlayer[playerID] = uid
	}
	s, c := prepare(playerID)
	if c != code.OK {
		return Result{}, c
	}
	if !knownSlot(slotID) {
		return Result{}, code.EquipSlotInvalid
	}
	cur := s.slots[slotID]
	if cur.item < 1 {
		return Result{}, code.EquipNotWorn
	}
	if cur.level >= maxLevel {
		return Result{}, code.EquipLevelMax
	}
	rule, ok := gcruntime.EnhanceRuleOf(cur.level + 1)
	if !ok {
		return Result{}, code.EquipInvalid
	}
	if mat.Count(stoneItem) < rule.StoneCount {
		return Result{}, code.BagItemNotEnough
	}
	if useProtect && mat.Count(protectItem) < 1 {
		return Result{}, code.BagItemNotEnough
	}
	success := rollFn(rule.SuccessPercent)
	dropped := false
	protectUsed := false
	level := cur.level
	if success {
		level++
	} else if rollFn(rule.DropPercent) {
		if useProtect {
			protectUsed = true
		} else {
			dropped = true
			level--
			if level < 0 {
				level = 0
			}
		}
	}
	if err := mat.Take(stoneItem, rule.StoneCount); err != nil {
		return Result{}, bagCode(err)
	}
	if protectUsed {
		if err := mat.Take(protectItem, 1); err != nil {
			return Result{}, bagCode(err)
		}
	}
	if level != cur.level {
		cur.level = level
		s.slots[slotID] = cur
		s.dirty = true
		syncCombat(playerID)
	}
	return Result{
		Success: success, Dropped: dropped, ProtectUsed: protectUsed,
		Level: level, Attrs: attrProto(cur.item, level),
	}, code.OK
}

func knownSlot(id int32) bool {
	for _, slot := range gcruntime.EquipSlots() {
		if slot.ID == id {
			return true
		}
	}
	return false
}

// boosted 在原数值上追加 max(1, 原数值 × 等级 / 10)。0 级不加。
func boosted(value, level int32) int32 {
	if level < 1 || value == 0 {
		return value
	}
	extra := int64(value) * int64(level) / 10
	if extra < 1 {
		extra = 1
	}
	sum := int64(value) + extra
	if sum > math.MaxInt32 {
		return math.MaxInt32
	}
	return int32(sum)
}
