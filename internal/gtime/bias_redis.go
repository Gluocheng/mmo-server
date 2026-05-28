package gtime

import (
	"context"
	"strconv"

	"github.com/redis/go-redis/v9"
)

// BiasRedisKey 时间偏置在 Redis 中的键。
func BiasRedisKey(keyPrefix string) string {
	return keyPrefix + ":meta:time_bias"
}

// LoadBiasFromRedis 从 Redis 加载偏置秒数；缺失或解析失败则保持 0。
func LoadBiasFromRedis(ctx context.Context, client *redis.Client, keyPrefix string) {
	if client == nil {
		return
	}
	raw, err := client.Get(ctx, BiasRedisKey(keyPrefix)).Result()
	if err != nil {
		return
	}
	sec, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || sec < 0 {
		return
	}
	SetBiasSeconds(sec)
}

// SaveBiasToRedis 将偏置秒数写入 Redis（TTL 0 表示不过期）。
func SaveBiasToRedis(ctx context.Context, client *redis.Client, keyPrefix string, sec int64) error {
	if client == nil {
		return nil
	}
	SetBiasSeconds(sec)
	return client.Set(ctx, BiasRedisKey(keyPrefix), strconv.FormatInt(sec, 10), 0).Err()
}
