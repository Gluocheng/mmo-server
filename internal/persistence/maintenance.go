package persistence

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/redis/go-redis/v9"
)

// MaintenanceState 全服维护旗，存 Redis。
type MaintenanceState struct {
	Enabled bool   `json:"enabled"`
	Reason  string `json:"reason"`
}

// MaintenanceRedisKey 维护旗 Redis 键。
func MaintenanceRedisKey() string {
	return KeyPrefix() + ":meta:maintenance"
}

// GetMaintenance 读取维护旗；键不存在或 Redis 未初始化视为未开启。
func GetMaintenance(ctx context.Context) (MaintenanceState, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if rdb == nil {
		return MaintenanceState{}, nil
	}
	raw, err := rdb.Get(ctx, MaintenanceRedisKey()).Result()
	if err == redis.Nil {
		return MaintenanceState{}, nil
	}
	if err != nil {
		return MaintenanceState{}, err
	}
	var st MaintenanceState
	if err := json.Unmarshal([]byte(raw), &st); err != nil {
		return MaintenanceState{}, err
	}
	return st, nil
}

// SaveMaintenance 写入维护旗；关闭时清空原因。Redis 不可用则失败。
func SaveMaintenance(ctx context.Context, enabled bool, reason string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if rdb == nil {
		return fmt.Errorf("redis unavailable")
	}
	reason = strings.TrimSpace(reason)
	if !enabled {
		reason = ""
	}
	raw, err := json.Marshal(MaintenanceState{Enabled: enabled, Reason: reason})
	if err != nil {
		return err
	}
	return rdb.Set(ctx, MaintenanceRedisKey(), raw, 0).Err()
}

// IsMaintenance 是否处于维护（拒登）。
func IsMaintenance(ctx context.Context) (bool, error) {
	st, err := GetMaintenance(ctx)
	if err != nil {
		return false, err
	}
	return st.Enabled, nil
}
