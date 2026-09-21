package persistence

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/example/mmo-server/internal/gtime"
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
var ErrDeviceMismatch = errors.New("token device mismatch")

func newToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func IssueTokenPairContext(parent context.Context, uid int64, deviceID string) (accessToken string, accessExpireAt int64, refreshToken string, refreshExpireAt int64, err error) {
	if err = Init(); err != nil {
		return "", 0, "", 0, err
	}
	if uid < 1 {
		return "", 0, "", 0, fmt.Errorf("uid invalid")
	}
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return "", 0, "", 0, fmt.Errorf("device id empty")
	}

	ctx, cancel := opContext(parent)
	defer cancel()
	return issueTokenPairWithUID(ctx, uid, deviceID)
}

func IssueTokenPair(uid int64, deviceID string) (accessToken string, accessExpireAt int64, refreshToken string, refreshExpireAt int64, err error) {
	return IssueTokenPairContext(context.Background(), uid, deviceID)
}

func VerifyAccessTokenContext(parent context.Context, token, requestDeviceID string) (int64, string, error) {
	if err := Init(); err != nil {
		return 0, "", err
	}
	token = strings.TrimSpace(token)
	requestDeviceID = strings.TrimSpace(requestDeviceID)
	if token == "" {
		return 0, "", fmt.Errorf("token empty")
	}
	ctx, cancel := opContext(parent)
	defer cancel()
	raw, err := rdb.Get(ctx, accessTokenKey(token)).Result()
	if err == redis.Nil {
		return 0, "", ErrAccessTokenInvalid
	}
	if err != nil {
		return 0, "", err
	}
	uid, tokenDeviceID, err := parseTokenValue(raw)
	if err != nil {
		return 0, "", err
	}
	if requestDeviceID != "" && requestDeviceID != tokenDeviceID {
		return 0, "", ErrDeviceMismatch
	}
	return uid, tokenDeviceID, nil
}

func VerifyAccessToken(token, requestDeviceID string) (int64, string, error) {
	return VerifyAccessTokenContext(context.Background(), token, requestDeviceID)
}

// PeekRefreshTokenUID 读取 refresh token 对应 uid，不消费该 token。
func PeekRefreshTokenUID(refreshToken string) (int64, error) {
	return PeekRefreshTokenUIDContext(context.Background(), refreshToken)
}

// PeekRefreshTokenUIDContext 读取 refresh token 对应 uid。
func PeekRefreshTokenUIDContext(parent context.Context, refreshToken string) (int64, error) {
	if err := Init(); err != nil {
		return 0, err
	}
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return 0, ErrRefreshTokenInvalid
	}
	if rdb == nil {
		return 0, fmt.Errorf("redis unavailable")
	}
	ctx, cancel := opContext(parent)
	defer cancel()
	raw, err := rdb.Get(ctx, refreshTokenKey(refreshToken)).Result()
	if err == redis.Nil {
		return 0, ErrRefreshTokenInvalid
	}
	if err != nil {
		return 0, err
	}
	uid, _, err := parseTokenValue(raw)
	if err != nil || uid < 1 {
		return 0, ErrRefreshTokenInvalid
	}
	return uid, nil
}

func RotateTokenPairByRefreshTokenContext(parent context.Context, refreshToken string) (accessToken string, accessExpireAt int64, newRefreshToken string, refreshExpireAt int64, deviceID string, err error) {
	if err = Init(); err != nil {
		return "", 0, "", 0, "", err
	}
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return "", 0, "", 0, "", fmt.Errorf("refresh token empty")
	}

	ctx, cancel := opContext(parent)
	defer cancel()
	raw, err := rdb.GetDel(ctx, refreshTokenKey(refreshToken)).Result()
	if err == redis.Nil {
		if used, usedErr := rdb.Exists(ctx, refreshUsedKey(refreshToken)).Result(); usedErr == nil && used > 0 {
			return "", 0, "", 0, "", ErrRefreshTokenReplay
		}
		return "", 0, "", 0, "", ErrRefreshTokenInvalid
	}
	if err != nil {
		return "", 0, "", 0, "", err
	}
	_ = rdb.Set(ctx, refreshUsedKey(refreshToken), "1", RefreshTTL()).Err()

	uid, deviceID, err := parseTokenValue(raw)
	if err != nil || uid < 1 {
		return "", 0, "", 0, "", fmt.Errorf("refresh token uid invalid")
	}
	indexRemoveMembers(ctx, uid, refreshIndexMember(refreshToken))

	accessToken, accessExpireAt, newRefreshToken, refreshExpireAt, err = issueTokenPairWithUID(ctx, uid, deviceID)
	if err != nil {
		return "", 0, "", 0, "", err
	}
	return accessToken, accessExpireAt, newRefreshToken, refreshExpireAt, deviceID, nil
}

func RotateTokenPairByRefreshToken(refreshToken string) (accessToken string, accessExpireAt int64, newRefreshToken string, refreshExpireAt int64, deviceID string, err error) {
	return RotateTokenPairByRefreshTokenContext(context.Background(), refreshToken)
}

func issueTokenPairWithUID(ctx context.Context, uid int64, deviceID string) (accessToken string, accessExpireAt int64, refreshToken string, refreshExpireAt int64, err error) {
	accessToken, err = newToken()
	if err != nil {
		return "", 0, "", 0, err
	}
	refreshToken, err = newToken()
	if err != nil {
		return "", 0, "", 0, err
	}
	accessExpireAt = gtime.Now().Add(AccessTTL()).Unix()
	refreshExpireAt = gtime.Now().Add(RefreshTTL()).Unix()
	tokenValue := buildTokenValue(uid, deviceID)
	if err := rdb.Set(ctx, accessTokenKey(accessToken), tokenValue, AccessTTL()).Err(); err != nil {
		return "", 0, "", 0, err
	}
	if err := rdb.Set(ctx, refreshTokenKey(refreshToken), tokenValue, RefreshTTL()).Err(); err != nil {
		_ = rdb.Del(ctx, accessTokenKey(accessToken)).Err()
		return "", 0, "", 0, err
	}
	if err := indexAddTokenPair(ctx, uid, accessToken, refreshToken); err != nil {
		_ = rdb.Del(ctx, accessTokenKey(accessToken), refreshTokenKey(refreshToken)).Err()
		return "", 0, "", 0, err
	}
	return accessToken, accessExpireAt, refreshToken, refreshExpireAt, nil
}

func buildTokenValue(uid int64, deviceID string) string {
	return strconv.FormatInt(uid, 10) + "|" + strings.TrimSpace(deviceID)
}

func parseTokenValue(raw string) (int64, string, error) {
	parts := strings.SplitN(raw, "|", 2)
	if len(parts) != 2 {
		return 0, "", fmt.Errorf("token value invalid")
	}
	uid, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || uid < 1 {
		return 0, "", fmt.Errorf("token uid invalid")
	}
	deviceID := strings.TrimSpace(parts[1])
	if deviceID == "" {
		return 0, "", fmt.Errorf("token device empty")
	}
	return uid, deviceID, nil
}

func RevokeTokensContext(parent context.Context, accessToken, refreshToken string) error {
	if err := Init(); err != nil {
		return err
	}
	accessToken = strings.TrimSpace(accessToken)
	refreshToken = strings.TrimSpace(refreshToken)
	if accessToken == "" && refreshToken == "" {
		return fmt.Errorf("token empty")
	}
	ctx, cancel := opContext(parent)
	defer cancel()
	uid := tokenUIDFromKeys(ctx, accessToken, refreshToken)
	keys := make([]string, 0, 2)
	if accessToken != "" {
		keys = append(keys, accessTokenKey(accessToken))
	}
	if refreshToken != "" {
		keys = append(keys, refreshTokenKey(refreshToken))
	}
	if err := rdb.Del(ctx, keys...).Err(); err != nil {
		return err
	}
	indexRemoveMembers(ctx, uid, accessIndexMember(accessToken), refreshIndexMember(refreshToken))
	return nil
}

func tokenUIDFromKeys(ctx context.Context, accessToken, refreshToken string) int64 {
	if accessToken != "" {
		if uid := peekUIDAt(ctx, accessTokenKey(accessToken)); uid > 0 {
			return uid
		}
	}
	if refreshToken != "" {
		return peekUIDAt(ctx, refreshTokenKey(refreshToken))
	}
	return 0
}

func peekUIDAt(ctx context.Context, key string) int64 {
	raw, err := rdb.Get(ctx, key).Result()
	if err != nil {
		return 0
	}
	uid, _, err := parseTokenValue(raw)
	if err != nil {
		return 0
	}
	return uid
}

func uidTokensKey(uid int64) string {
	return fmt.Sprintf("%s:tokens:uid:%d", KeyPrefix(), uid)
}

func accessIndexMember(token string) string {
	if token == "" {
		return ""
	}
	return "a:" + token
}

func refreshIndexMember(token string) string {
	if token == "" {
		return ""
	}
	return "r:" + token
}

func indexAddTokenPair(ctx context.Context, uid int64, accessToken, refreshToken string) error {
	if rdb == nil || uid < 1 {
		return nil
	}
	members := make([]any, 0, 2)
	if accessToken != "" {
		members = append(members, accessIndexMember(accessToken))
	}
	if refreshToken != "" {
		members = append(members, refreshIndexMember(refreshToken))
	}
	if len(members) == 0 {
		return nil
	}
	key := uidTokensKey(uid)
	if err := rdb.SAdd(ctx, key, members...).Err(); err != nil {
		return err
	}
	ttl := RefreshTTL()
	if ttl <= 0 {
		return nil
	}
	return rdb.Expire(ctx, key, ttl).Err()
}

func indexRemoveMembers(ctx context.Context, uid int64, members ...string) {
	if rdb == nil || uid < 1 {
		return
	}
	args := make([]any, 0, len(members))
	for _, m := range members {
		if m != "" {
			args = append(args, m)
		}
	}
	if len(args) == 0 {
		return
	}
	_ = rdb.SRem(ctx, uidTokensKey(uid), args...).Err()
}

// RevokeAllTokensForUID 删除该 uid 索引内全部 access/refresh（封号用）。
func RevokeAllTokensForUID(uid int64) error {
	return RevokeAllTokensForUIDContext(context.Background(), uid)
}

// RevokeAllTokensForUIDContext 按 uid 反向索引批量吊销令牌。
func RevokeAllTokensForUIDContext(parent context.Context, uid int64) error {
	if uid < 1 {
		return nil
	}
	if rdb == nil {
		if err := Init(); err != nil {
			return err
		}
	}
	if rdb == nil {
		return nil
	}
	ctx, cancel := opContext(parent)
	defer cancel()
	key := uidTokensKey(uid)
	members, err := rdb.SMembers(ctx, key).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return err
	}
	delKeys := make([]string, 0, len(members)+1)
	for _, m := range members {
		switch {
		case strings.HasPrefix(m, "a:"):
			delKeys = append(delKeys, accessTokenKey(strings.TrimPrefix(m, "a:")))
		case strings.HasPrefix(m, "r:"):
			tok := strings.TrimPrefix(m, "r:")
			delKeys = append(delKeys, refreshTokenKey(tok), refreshUsedKey(tok))
		}
	}
	delKeys = append(delKeys, key)
	return rdb.Del(ctx, delKeys...).Err()
}

func RevokeTokens(accessToken, refreshToken string) error {
	return RevokeTokensContext(context.Background(), accessToken, refreshToken)
}
