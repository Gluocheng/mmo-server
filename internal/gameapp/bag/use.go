package bag

import (
	"github.com/example/mmo-server/internal/code"
	"github.com/example/mmo-server/internal/gameapp/combat"
)

// ApplyUse 按道具配置的 use_buff_id 挂 Buff。返回非 OK 时调用方不得扣道具。
func ApplyUse(uid int64, useBuffID int32) int32 {
	if useBuffID < 1 {
		return code.ItemNotUsable
	}
	return combat.ApplyBuff(uid, useBuffID)
}
