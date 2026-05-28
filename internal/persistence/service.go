package persistence

import (
	"context"
	"fmt"
	"strings"

	"github.com/example/mmo-server/internal/protocol"
)

// LoginOrCreateAccount 登录或注册账号，MySQL 写操作在统一事务内完成，缓存于提交后刷新。
func LoginOrCreateAccountContext(parent context.Context, nickname, password string) (int64, error) {
	if err := ensureDB(); err != nil {
		return 0, err
	}
	nickname = strings.TrimSpace(nickname)
	password = strings.TrimSpace(password)
	if nickname == "" || password == "" {
		return 0, fmt.Errorf("nickname or password empty")
	}

	ctx, cancel := opContext(parent)
	defer cancel()

	cacheKey := accountNicknameKey(nickname)
	if rdb != nil {
		if cached, err := rdb.Get(ctx, cacheKey).Result(); err == nil {
			if uid, hash, ok := parseAccountCacheValue(cached); ok {
				if verifyPassword(hash, password) {
					return uid, nil
				}
				return 0, ErrInvalidPassword
			}
		}
	}

	var uid int64
	err := WithinTx(ctx, func(txCtx context.Context) error {
		var innerErr error
		uid, innerErr = findOrCreateAccountInTx(txCtx, nickname, password)
		return innerErr
	})
	return uid, err
}

func LoginOrCreateAccount(nickname, password string) (int64, error) {
	return LoginOrCreateAccountContext(context.Background(), nickname, password)
}

// CreatePlayerForUID 为账号创建角色，MySQL 写操作在统一事务内完成，缓存于提交后刷新。
func CreatePlayerForUIDContext(parent context.Context, uid int64, name string) (*protocol.PlayerInfo, bool, error) {
	if err := ensureDB(); err != nil {
		return nil, false, err
	}
	name = strings.TrimSpace(name)
	if uid < 1 || name == "" {
		return nil, false, nil
	}

	ctx, cancel := opContext(parent)
	defer cancel()

	var (
		info    *protocol.PlayerInfo
		created bool
	)
	err := WithinTx(ctx, func(txCtx context.Context) error {
		var innerErr error
		info, created, innerErr = createPlayerInTx(txCtx, uid, name)
		return innerErr
	})
	return info, created, err
}

func CreatePlayerForUID(uid int64, name string) (*protocol.PlayerInfo, bool, error) {
	return CreatePlayerForUIDContext(context.Background(), uid, name)
}
