package persistence

import (
	"context"
	"fmt"
	"strings"
	"time"
)

func normalizeIP(ip string) string {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return "unknown"
	}
	return ip
}

func normalizeNickname(nickname string) string {
	nickname = strings.ToLower(strings.TrimSpace(nickname))
	if nickname == "" {
		return "unknown"
	}
	return nickname
}

func loginFailKey(ip, nickname string) string {
	return fmt.Sprintf("%s:login:fail:%s:%s", KeyPrefix(), normalizeIP(ip), normalizeNickname(nickname))
}

func loginBlockKey(ip, nickname string) string {
	return fmt.Sprintf("%s:login:block:%s:%s", KeyPrefix(), normalizeIP(ip), normalizeNickname(nickname))
}

func IsLoginBlocked(ip, nickname string) (bool, error) {
	if err := Init(); err != nil {
		return false, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	n, err := rdb.Exists(ctx, loginBlockKey(ip, nickname)).Result()
	return n > 0, err
}

func RecordLoginFailure(ip, nickname string) error {
	if err := Init(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	failKey := loginFailKey(ip, nickname)
	count, err := rdb.Incr(ctx, failKey).Result()
	if err != nil {
		return err
	}
	if count == 1 {
		_ = rdb.Expire(ctx, failKey, LoginFailWindow()).Err()
	}
	if int(count) >= LoginFailLimit() {
		_ = rdb.Set(ctx, loginBlockKey(ip, nickname), "1", LoginBlockTTL()).Err()
	}
	return nil
}

func ClearLoginFailure(ip, nickname string) error {
	if err := Init(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return rdb.Del(ctx, loginFailKey(ip, nickname), loginBlockKey(ip, nickname)).Err()
}
