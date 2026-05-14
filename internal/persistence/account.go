package persistence

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var ErrInvalidPassword = errors.New("invalid password")

func accountNicknameKey(nickname string) string {
	return fmt.Sprintf("%s:account:nickname:%s", KeyPrefix(), strings.ToLower(nickname))
}

func hashPassword(password string) (string, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashed), nil
}

func verifyPassword(hashed, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hashed), []byte(password)) == nil
}

func accountCacheValue(uid int64, passwordHash string) string {
	return strconv.FormatInt(uid, 10) + "|" + passwordHash
}

func parseAccountCacheValue(v string) (int64, string, bool) {
	parts := strings.SplitN(v, "|", 2)
	if len(parts) != 2 {
		return 0, "", false
	}
	uid, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || uid < 1 {
		return 0, "", false
	}
	return uid, parts[1], true
}

func FindOrCreateAccount(nickname, password string) (int64, error) {
	if err := Init(); err != nil {
		return 0, err
	}
	nickname = strings.TrimSpace(nickname)
	password = strings.TrimSpace(password)
	if nickname == "" || password == "" {
		return 0, fmt.Errorf("nickname or password empty")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cacheKey := accountNicknameKey(nickname)
	if cached, err := rdb.Get(ctx, cacheKey).Result(); err == nil {
		if uid, hash, ok := parseAccountCacheValue(cached); ok {
			if verifyPassword(hash, password) {
				return uid, nil
			}
			return 0, ErrInvalidPassword
		}
	}

	var acc Account
	err := db.WithContext(ctx).Where("nickname = ?", nickname).First(&acc).Error
	if err == nil {
		if !verifyPassword(acc.Password, password) {
			return 0, ErrInvalidPassword
		}
		_ = rdb.Set(ctx, cacheKey, accountCacheValue(acc.UID, acc.Password), CacheTTL()).Err()
		return acc.UID, nil
	}
	if err != gorm.ErrRecordNotFound {
		return 0, err
	}

	passwordHash, err := hashPassword(password)
	if err != nil {
		return 0, err
	}
	acc = Account{
		Nickname: nickname,
		Password: passwordHash,
	}
	if err = db.WithContext(ctx).Create(&acc).Error; err != nil {
		// 并发插入可能触发唯一键冲突，兜底再查一次
		var existed Account
		qErr := db.WithContext(ctx).Where("nickname = ?", nickname).First(&existed).Error
		if qErr != nil {
			return 0, err
		}
		if !verifyPassword(existed.Password, password) {
			return 0, ErrInvalidPassword
		}
		acc = existed
	}

	_ = rdb.Set(ctx, cacheKey, accountCacheValue(acc.UID, acc.Password), CacheTTL()).Err()
	return acc.UID, nil
}
