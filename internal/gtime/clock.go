// Package gtime 提供游戏服统一时间入口（当前时刻 + 日历边界/跨期判断）。
// 业务代码请使用 Now/UnixNow，避免直接 time.Now()，以便测试偏置与 Cherry 全局偏移一致。
package gtime

import (
	"sync/atomic"
	"time"

	cherryTime "github.com/cherry-game/cherry/extend/time"
)

var biasSeconds atomic.Int64

// SetBiasSeconds 设置进程内时间偏置（秒，仅向前）。同时写入 Cherry 全局 offset。
func SetBiasSeconds(sec int64) {
	if sec < 0 {
		sec = 0
	}
	biasSeconds.Store(sec)
	cherryTime.AddOffsetTime(time.Duration(sec) * time.Second)
}

// BiasSeconds 返回当前偏置秒数。
func BiasSeconds() int64 {
	return biasSeconds.Load()
}

// HasBias 是否启用了时间偏置。
func HasBias() bool {
	return biasSeconds.Load() > 0
}

// Now 返回游戏逻辑使用的当前时间（含偏置）。
func Now() time.Time {
	return cherryTime.Now().Time
}

// UnixNow 返回游戏逻辑使用的当前 Unix 秒。
func UnixNow() int64 {
	return Now().Unix()
}

// RealNow 返回真实系统时间（不受偏置影响）。
func RealNow() time.Time {
	return time.Now()
}
