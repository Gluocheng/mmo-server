package persistence

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	clog "github.com/cherry-game/cherry/logger"
	"github.com/example/mmo-server/internal/code"
	"github.com/example/mmo-server/internal/persistence/model"
	"gorm.io/gorm"
)

const (
	// GMRoleAdmin 可开号、禁用、改密。
	GMRoleAdmin = model.GMRoleAdmin
	// GMRoleOperator 与管理员玩法权限相同，不能管号。
	GMRoleOperator = model.GMRoleOperator
	// GMSystemOperator 共享 token 机器调用写入审计的操作者名。
	GMSystemOperator = "system"
	// GMSessionTTL 控制台 Cookie 会话有效期。
	GMSessionTTL     = 12 * time.Hour
	gmMinPasswordLen = 6
)

var (
	ErrGMUserNotFound = errors.New("gm user not found")
	ErrGMUserDisabled = errors.New("gm user disabled")
	ErrGMUserExists   = errors.New("gm user exists")
	ErrGMLastAdmin    = errors.New("cannot disable last admin")
	ErrGMInvalidRole  = errors.New("invalid gm role")
	ErrGMWeakPassword = errors.New("password too short")
	ErrGMSessionGone  = errors.New("gm session not found")
)

// GMUserView 对外用户视图，不含密码。
type GMUserView struct {
	ID            int64  `json:"id"`
	Username      string `json:"username"`
	DisplayName   string `json:"displayName"`
	Role          string `json:"role"`
	Disabled      bool   `json:"disabled"`
	CreatedAtUnix int64  `json:"createdAtUnix"`
}

// GMSession 存在 Redis 或测试内存中的登录态。
type GMSession struct {
	UserID      int64  `json:"userId"`
	Username    string `json:"username"`
	Role        string `json:"role"`
	DisplayName string `json:"displayName"`
}

type memSess struct {
	raw string
	exp time.Time
}

var (
	gmSessMu  sync.Mutex
	gmSessMem = map[string]memSess{}
)

func gmKeyPrefix() string {
	p := KeyPrefix()
	if p == "" {
		return "mmo"
	}
	return p
}

func gmUserView(u model.GMUser) GMUserView {
	return GMUserView{
		ID:            u.ID,
		Username:      u.Username,
		DisplayName:   u.DisplayName,
		Role:          u.Role,
		Disabled:      u.Disabled,
		CreatedAtUnix: u.CreatedAt.Unix(),
	}
}

func validGMRole(role string) bool {
	return role == GMRoleAdmin || role == GMRoleOperator
}

// EnsureGMBootstrap 仅在 gm_users 为空且用户名密码非空时插入管理员。
func EnsureGMBootstrap(username, password string) error {
	username = strings.TrimSpace(username)
	password = strings.TrimSpace(password)
	if username == "" || password == "" {
		return nil
	}
	if err := ensureDB(); err != nil {
		return err
	}
	var n int64
	if err := db.Model(&model.GMUser{}).Count(&n).Error; err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	_, err := CreateGMUser(username, password, username, GMRoleAdmin)
	if err != nil {
		return err
	}
	clog.Infof("gm: bootstrapped admin user %s", username)
	return nil
}

// CreateGMUser 创建运营账号。
func CreateGMUser(username, password, displayName, role string) (*GMUserView, error) {
	if err := ensureDB(); err != nil {
		return nil, err
	}
	username = strings.TrimSpace(username)
	displayName = strings.TrimSpace(displayName)
	role = strings.TrimSpace(role)
	if username == "" {
		return nil, fmt.Errorf("username empty")
	}
	if len(password) < gmMinPasswordLen {
		return nil, ErrGMWeakPassword
	}
	if !validGMRole(role) {
		return nil, ErrGMInvalidRole
	}
	if displayName == "" {
		displayName = username
	}
	hash, err := hashPassword(password)
	if err != nil {
		return nil, err
	}
	u := model.GMUser{
		Username:     username,
		PasswordHash: hash,
		DisplayName:  displayName,
		Role:         role,
	}
	if err := db.Create(&u).Error; err != nil {
		if isUniqueViolation(err) {
			return nil, ErrGMUserExists
		}
		return nil, err
	}
	v := gmUserView(u)
	return &v, nil
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "unique") || strings.Contains(s, "duplicate")
}

// ListGMUsers 列出全部运营账号（不含密码）。
func ListGMUsers() ([]GMUserView, error) {
	if err := ensureDB(); err != nil {
		return nil, err
	}
	var rows []model.GMUser
	if err := db.Order("id asc").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]GMUserView, 0, len(rows))
	for i := range rows {
		out = append(out, gmUserView(rows[i]))
	}
	return out, nil
}

// AuthenticateGMUser 校验密码；禁用账号视为失败。
func AuthenticateGMUser(username, password string) (*GMUserView, error) {
	if err := ensureDB(); err != nil {
		return nil, err
	}
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return nil, ErrInvalidPassword
	}
	var u model.GMUser
	err := db.Where("username = ?", username).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrInvalidPassword
	}
	if err != nil {
		return nil, err
	}
	if u.Disabled {
		return nil, ErrGMUserDisabled
	}
	if !verifyPassword(u.PasswordHash, password) {
		return nil, ErrInvalidPassword
	}
	v := gmUserView(u)
	return &v, nil
}

// SetGMUserDisabled 禁用或启用；不能禁用最后一个未禁用 admin。
func SetGMUserDisabled(id int64, disabled bool) error {
	if err := ensureDB(); err != nil {
		return err
	}
	var u model.GMUser
	if err := db.First(&u, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrGMUserNotFound
		}
		return err
	}
	if disabled && u.Role == GMRoleAdmin && !u.Disabled {
		var n int64
		if err := db.Model(&model.GMUser{}).
			Where("role = ? AND disabled = ?", GMRoleAdmin, false).
			Count(&n).Error; err != nil {
			return err
		}
		if n <= 1 {
			return ErrGMLastAdmin
		}
	}
	return db.Model(&u).Update("disabled", disabled).Error
}

// SetGMUserPassword 管理员重置密码。
func SetGMUserPassword(id int64, password string) error {
	if err := ensureDB(); err != nil {
		return err
	}
	if len(password) < gmMinPasswordLen {
		return ErrGMWeakPassword
	}
	var u model.GMUser
	if err := db.First(&u, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrGMUserNotFound
		}
		return err
	}
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	return db.Model(&u).Update("password_hash", hash).Error
}

func gmSessKey(token string) string {
	return fmt.Sprintf("%s:gm:sess:%s", gmKeyPrefix(), token)
}

func gmLoginFailKey(username string) string {
	return fmt.Sprintf("%s:gm:login:fail:%s", gmKeyPrefix(), strings.ToLower(strings.TrimSpace(username)))
}

func gmLoginBlockKey(username string) string {
	return fmt.Sprintf("%s:gm:login:block:%s", gmKeyPrefix(), strings.ToLower(strings.TrimSpace(username)))
}

// NewGMSessionToken 生成随机会话 ID。
func NewGMSessionToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// PutGMSession 写入会话；无 Redis 时用进程内存（单测）。
func PutGMSession(token string, sess GMSession, ttl time.Duration) error {
	if token == "" {
		return fmt.Errorf("empty session token")
	}
	if ttl <= 0 {
		ttl = GMSessionTTL
	}
	raw, err := json.Marshal(sess)
	if err != nil {
		return err
	}
	if rdb != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		return rdb.Set(ctx, gmSessKey(token), raw, ttl).Err()
	}
	gmSessMu.Lock()
	gmSessMem[token] = memSess{raw: string(raw), exp: time.Now().Add(ttl)}
	gmSessMu.Unlock()
	return nil
}

// GetGMSession 读取未过期会话。
func GetGMSession(token string) (*GMSession, error) {
	if token == "" {
		return nil, ErrGMSessionGone
	}
	var raw string
	if rdb != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		s, err := rdb.Get(ctx, gmSessKey(token)).Result()
		if err != nil {
			return nil, ErrGMSessionGone
		}
		raw = s
	} else {
		gmSessMu.Lock()
		ent, ok := gmSessMem[token]
		if !ok || time.Now().After(ent.exp) {
			if ok {
				delete(gmSessMem, token)
			}
			gmSessMu.Unlock()
			return nil, ErrGMSessionGone
		}
		raw = ent.raw
		gmSessMu.Unlock()
	}
	var sess GMSession
	if err := json.Unmarshal([]byte(raw), &sess); err != nil {
		return nil, err
	}
	return &sess, nil
}

// DeleteGMSession 注销会话。
func DeleteGMSession(token string) {
	if token == "" {
		return
	}
	if rdb != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = rdb.Del(ctx, gmSessKey(token)).Err()
		return
	}
	gmSessMu.Lock()
	delete(gmSessMem, token)
	gmSessMu.Unlock()
}

// IsGMLoginBlocked 用户名是否因失败过多被封。无 Redis 时不拦截。
func IsGMLoginBlocked(username string) bool {
	if rdb == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	n, err := rdb.Exists(ctx, gmLoginBlockKey(username)).Result()
	return err == nil && n > 0
}

// RecordGMLoginFailure 累计失败；达阈值后封禁。无 Redis 时忽略。
func RecordGMLoginFailure(username string) {
	if rdb == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	key := gmLoginFailKey(username)
	count, err := rdb.Incr(ctx, key).Result()
	if err != nil {
		return
	}
	if count == 1 {
		_ = rdb.Expire(ctx, key, LoginFailWindow()).Err()
	}
	limit := LoginFailLimit()
	if limit < 1 {
		limit = 5
	}
	if int(count) >= limit {
		block := LoginBlockTTL()
		if block <= 0 {
			block = 10 * time.Minute
		}
		_ = rdb.Set(ctx, gmLoginBlockKey(username), "1", block).Err()
	}
}

// ClearGMLoginFailure 登录成功后清失败计数。
func ClearGMLoginFailure(username string) {
	if rdb == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = rdb.Del(ctx, gmLoginFailKey(username), gmLoginBlockKey(username)).Err()
}

// GMOpLogView 审计列表项。
type GMOpLogView struct {
	ID             int64  `json:"id"`
	Operator       string `json:"operator"`
	Action         string `json:"action"`
	TargetUID      int64  `json:"targetUid"`
	TargetPlayerID int64  `json:"targetPlayerId"`
	Detail         string `json:"detail"`
	ResultCode     int32  `json:"resultCode"`
	CreatedAtUnix  int64  `json:"createdAtUnix"`
}

// GMOpLogFilter 操作日志筛选。
type GMOpLogFilter struct {
	Operator string
	Action   string
	FromUnix int64
	ToUnix   int64
	Page     int
	PageSize int
}

// ListGMOpLogs 分页查询审计；page 从 1 起，pageSize 默认 20、最大 100。
func ListGMOpLogs(f GMOpLogFilter) (list []GMOpLogView, total int64, page, pageSize int, err error) {
	if err = ensureDB(); err != nil {
		return nil, 0, 0, 0, err
	}
	page = f.Page
	if page < 1 {
		page = 1
	}
	pageSize = f.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	q := db.Model(&model.GMOpLog{})
	if op := strings.TrimSpace(f.Operator); op != "" {
		q = q.Where("operator = ?", op)
	}
	if act := strings.TrimSpace(f.Action); act != "" {
		q = q.Where("action = ?", act)
	}
	if f.FromUnix > 0 {
		q = q.Where("created_at >= ?", time.Unix(f.FromUnix, 0))
	}
	if f.ToUnix > 0 {
		q = q.Where("created_at <= ?", time.Unix(f.ToUnix, 0))
	}
	if err = q.Count(&total).Error; err != nil {
		return nil, 0, page, pageSize, err
	}
	var rows []model.GMOpLog
	offset := (page - 1) * pageSize
	if err = q.Order("id desc").Offset(offset).Limit(pageSize).Find(&rows).Error; err != nil {
		return nil, 0, page, pageSize, err
	}
	list = make([]GMOpLogView, 0, len(rows))
	for i := range rows {
		r := rows[i]
		list = append(list, GMOpLogView{
			ID:             r.ID,
			Operator:       r.Operator,
			Action:         r.Action,
			TargetUID:      r.TargetUID,
			TargetPlayerID: r.TargetPlayerID,
			Detail:         r.Detail,
			ResultCode:     r.ResultCode,
			CreatedAtUnix:  r.CreatedAt.Unix(),
		})
	}
	return list, total, page, pageSize, nil
}

// GMAuthFailCode 将账号校验错误映射为业务码。
func GMAuthFailCode(err error) int32 {
	switch {
	case errors.Is(err, ErrGMUserDisabled):
		return code.GmForbidden
	case errors.Is(err, ErrInvalidPassword), errors.Is(err, ErrGMSessionGone):
		return code.GmUnauthorized
	default:
		return code.LoginFail
	}
}
