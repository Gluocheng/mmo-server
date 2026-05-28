package persistence

import (
	"context"
	"fmt"
	"strings"
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

func IsLoginBlockedContext(parent context.Context, ip, nickname string) (bool, error) {
	if err := Init(); err != nil {
		return false, err
	}
	ctx, cancel := opContext(parent)
	defer cancel()
	n, err := rdb.Exists(ctx, loginBlockKey(ip, nickname)).Result()
	return n > 0, err
}

func IsLoginBlocked(ip, nickname string) (bool, error) {
	return IsLoginBlockedContext(context.Background(), ip, nickname)
}

func RecordLoginFailureContext(parent context.Context, ip, nickname string) error {
	if err := Init(); err != nil {
		return err
	}
	ctx, cancel := opContext(parent)
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

func RecordLoginFailure(ip, nickname string) error {
	return RecordLoginFailureContext(context.Background(), ip, nickname)
}

func ClearLoginFailureContext(parent context.Context, ip, nickname string) error {
	if err := Init(); err != nil {
		return err
	}
	ctx, cancel := opContext(parent)
	defer cancel()
	return rdb.Del(ctx, loginFailKey(ip, nickname), loginBlockKey(ip, nickname)).Err()
}

func ClearLoginFailure(ip, nickname string) error {
	return ClearLoginFailureContext(context.Background(), ip, nickname)
}
