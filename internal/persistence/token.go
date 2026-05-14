package persistence

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

func accessTokenKey(token string) string {
	return fmt.Sprintf("%s:access:%s", KeyPrefix(), token)
}

func refreshTokenKey(token string) string {
	return fmt.Sprintf("%s:refresh:%s", KeyPrefix(), token)
}

func refreshUsedKey(token string) string {
	return fmt.Sprintf("%s:refresh_used:%s", KeyPrefix(), token)
}

var ErrRefreshTokenReplay = errors.New("refresh token replay detected")
var ErrAccessTokenInvalid = errors.New("access token invalid")
var ErrRefreshTokenInvalid = errors.New("refresh token invalid")

func newToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func IssueTokenPair(uid int64) (accessToken string, accessExpireAt int64, refreshToken string, refreshExpireAt int64, err error) {
	if err = Init(); err != nil {
		return "", 0, "", 0, err
	}
	if uid < 1 {
		return "", 0, "", 0, fmt.Errorf("uid invalid")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return issueTokenPairWithUID(ctx, uid)
}

func VerifyAccessToken(token string) (int64, error) {
	if err := Init(); err != nil {
		return 0, err
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return 0, fmt.Errorf("token empty")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	raw, err := rdb.Get(ctx, accessTokenKey(token)).Result()
	if err == redis.Nil {
		return 0, ErrAccessTokenInvalid
	}
	if err != nil {
		return 0, err
	}
	uid, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, err
	}
	return uid, nil
}

func RotateTokenPairByRefreshToken(refreshToken string) (accessToken string, accessExpireAt int64, newRefreshToken string, refreshExpireAt int64, err error) {
	if err = Init(); err != nil {
		return "", 0, "", 0, err
	}
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return "", 0, "", 0, fmt.Errorf("refresh token empty")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	raw, err := rdb.GetDel(ctx, refreshTokenKey(refreshToken)).Result()
	if err == redis.Nil {
		if used, usedErr := rdb.Exists(ctx, refreshUsedKey(refreshToken)).Result(); usedErr == nil && used > 0 {
			return "", 0, "", 0, ErrRefreshTokenReplay
		}
		return "", 0, "", 0, ErrRefreshTokenInvalid
	}
	if err != nil {
		return "", 0, "", 0, err
	}
	_ = rdb.Set(ctx, refreshUsedKey(refreshToken), "1", RefreshTTL()).Err()

	uid, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || uid < 1 {
		return "", 0, "", 0, fmt.Errorf("refresh token uid invalid")
	}

	accessToken, accessExpireAt, newRefreshToken, refreshExpireAt, err = issueTokenPairWithUID(ctx, uid)
	if err != nil {
		return "", 0, "", 0, err
	}
	return accessToken, accessExpireAt, newRefreshToken, refreshExpireAt, nil
}

func issueTokenPairWithUID(ctx context.Context, uid int64) (accessToken string, accessExpireAt int64, refreshToken string, refreshExpireAt int64, err error) {
	accessToken, err = newToken()
	if err != nil {
		return "", 0, "", 0, err
	}
	refreshToken, err = newToken()
	if err != nil {
		return "", 0, "", 0, err
	}
	accessExpireAt = time.Now().Add(AccessTTL()).Unix()
	refreshExpireAt = time.Now().Add(RefreshTTL()).Unix()
	if err := rdb.Set(ctx, accessTokenKey(accessToken), strconv.FormatInt(uid, 10), AccessTTL()).Err(); err != nil {
		return "", 0, "", 0, err
	}
	if err := rdb.Set(ctx, refreshTokenKey(refreshToken), strconv.FormatInt(uid, 10), RefreshTTL()).Err(); err != nil {
		_ = rdb.Del(ctx, accessTokenKey(accessToken)).Err()
		return "", 0, "", 0, err
	}
	return accessToken, accessExpireAt, refreshToken, refreshExpireAt, nil
}

func RevokeTokens(accessToken, refreshToken string) error {
	if err := Init(); err != nil {
		return err
	}
	accessToken = strings.TrimSpace(accessToken)
	refreshToken = strings.TrimSpace(refreshToken)
	if accessToken == "" && refreshToken == "" {
		return fmt.Errorf("token empty")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	keys := make([]string, 0, 2)
	if accessToken != "" {
		keys = append(keys, accessTokenKey(accessToken))
	}
	if refreshToken != "" {
		keys = append(keys, refreshTokenKey(refreshToken))
	}
	return rdb.Del(ctx, keys...).Err()
}
