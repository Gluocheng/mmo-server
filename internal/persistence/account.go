package persistence

import (
	"context"

	"errors"

	"fmt"

	"strconv"

	clog "github.com/cherry-game/cherry/logger"

	"strings"

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

func scheduleAccountCacheRefresh(ctx context.Context, nickname string, uid int64, passwordHash string) {
	if rdb == nil {
		return
	}

	cacheKey := accountNicknameKey(nickname)

	value := accountCacheValue(uid, passwordHash)

	AfterCommit(ctx, func(commitCtx context.Context) {

		if err := rdb.Set(commitCtx, cacheKey, value, CacheTTL()).Err(); err != nil {

			clog.Warnf("persistence: after commit account cache failed nickname=%s err=%v", nickname, err)

		}

	})

}

func findOrCreateAccountInTx(ctx context.Context, nickname, password string) (int64, error) {

	var acc Account

	err := DBFromContext(ctx).WithContext(ctx).Where("nickname = ?", nickname).First(&acc).Error

	if err == nil {

		if !verifyPassword(acc.Password, password) {

			return 0, ErrInvalidPassword

		}

		scheduleAccountCacheRefresh(ctx, nickname, acc.UID, acc.Password)

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

	if err = DBFromContext(ctx).WithContext(ctx).Create(&acc).Error; err != nil {

		var existed Account

		qErr := DBFromContext(ctx).WithContext(ctx).Where("nickname = ?", nickname).First(&existed).Error

		if qErr != nil {

			return 0, err

		}

		if !verifyPassword(existed.Password, password) {

			return 0, ErrInvalidPassword

		}

		acc = existed

	}

	scheduleAccountCacheRefresh(ctx, nickname, acc.UID, acc.Password)

	return acc.UID, nil

}

func FindOrCreateAccountContext(parent context.Context, nickname, password string) (int64, error) {
	return LoginOrCreateAccountContext(parent, nickname, password)
}

// FindOrCreateAccount 兼容入口，委托给 LoginOrCreateAccount。
func FindOrCreateAccount(nickname, password string) (int64, error) {
	return FindOrCreateAccountContext(context.Background(), nickname, password)
}
