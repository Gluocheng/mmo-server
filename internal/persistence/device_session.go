package persistence

import (
	"context"
	_ "embed"
	"fmt"
	"strings"
	"time"

	cprofile "github.com/cherry-game/cherry/profile"
	"github.com/redis/go-redis/v9"
)

func deviceSetKey(uid int64) string {
	return fmt.Sprintf("%s:session:uid:%d:devices", KeyPrefix(), uid)
}

//go:embed scripts/register_device.lua
var registerDeviceLua string

var registerDeviceScript = redis.NewScript(registerDeviceLua)

// RegisterDeviceSession 设备会话注册（原子检查并写入）
// 返回值: allowed=true 允许登录; false 表示超出设备数量限制
func RegisterDeviceSession(uid int64, deviceID string, maxDevices int) (bool, error) {
	if err := Init(); err != nil {
		return false, err
	}
	if uid < 1 {
		return false, fmt.Errorf("uid invalid")
	}
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return false, fmt.Errorf("device id empty")
	}
	if maxDevices < 1 {
		maxDevices = 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	key := deviceSetKey(uid)
	rsp, err := registerDeviceScript.Run(ctx, rdb, []string{key}, deviceID, maxDevices, int(DeviceSessionTTL().Seconds())).Int()
	if err != nil {
		return false, err
	}
	return rsp == 1, nil
}

func RemoveDeviceSession(uid int64, deviceID string) error {
	if err := Init(); err != nil {
		return err
	}
	if uid < 1 {
		return nil
	}
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	key := deviceSetKey(uid)
	if err := rdb.SRem(ctx, key, deviceID).Err(); err != nil {
		return err
	}
	card, err := rdb.SCard(ctx, key).Result()
	if err == nil && card == 0 {
		_ = rdb.Del(ctx, key).Err()
	}
	return nil
}

func SessionPolicyConfig() (policy string, maxDevices int) {
	auth := cprofile.GetConfig("auth")
	policy = strings.TrimSpace(auth.GetString("session_policy", "kick_old"))
	maxDevices = auth.GetInt("max_devices_per_uid", 2)
	if maxDevices < 1 {
		maxDevices = 1
	}
	return policy, maxDevices
}
