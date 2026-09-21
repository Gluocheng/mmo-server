package persistence

import (
	"context"
	"errors"
	"strings"

	clog "github.com/cherry-game/cherry/logger"
	"github.com/example/mmo-server/internal/gtime"
	"github.com/example/mmo-server/internal/persistence/model"
	"github.com/example/mmo-server/internal/protocol"
	"gorm.io/gorm"
)

const (
	GMActionReload      = "config.reload"
	GMActionGrant       = "bag.grant"
	GMActionKick        = "player.kick"
	GMActionDeduct      = "bag.deduct"
	GMActionBan         = "account.ban"
	GMActionUnban       = "account.unban"
	GMActionNotice      = "world.notice"
	GMActionTimeSet     = "time.set"
	GMActionMute        = "account.mute"
	GMActionUnmute      = "account.unmute"
	GMActionMaintenance = "world.maintenance"
)

func gmPlayerRecord(p model.Player) *protocol.GmPlayerRecord {
	rec := &protocol.GmPlayerRecord{
		PlayerId:      p.PlayerID,
		Uid:           p.UID,
		Name:          p.Name,
		Deleted:       p.DeletedAt != nil,
		CreatedAtUnix: p.CreatedAt.Unix(),
	}
	return rec
}

func gmAccountView(a model.Account) *protocol.GmAccountView {
	return &protocol.GmAccountView{
		Uid:           a.UID,
		Nickname:      a.Nickname,
		CreatedAtUnix: a.CreatedAt.Unix(),
		Banned:        a.Banned,
		BanReason:     a.BanReason,
		BannedUntil:   a.BannedUntil,
		Muted:         a.Muted,
		MuteReason:    a.MuteReason,
		MutedUntil:    a.MutedUntil,
	}
}

// applyAccountExpiry 惰性清除已到期的封禁/禁言，失败只打日志。
func applyAccountExpiry(ctx context.Context, acc *model.Account) {
	if acc == nil || acc.UID < 1 {
		return
	}
	now := gtime.UnixNow()
	updates := map[string]any{}
	if acc.Banned && acc.BannedUntil > 0 && now >= acc.BannedUntil {
		acc.Banned = false
		acc.BanReason = ""
		acc.BannedUntil = 0
		updates["banned"] = false
		updates["ban_reason"] = ""
		updates["banned_until"] = 0
	}
	if acc.Muted && acc.MutedUntil > 0 && now >= acc.MutedUntil {
		acc.Muted = false
		acc.MuteReason = ""
		acc.MutedUntil = 0
		updates["muted"] = false
		updates["mute_reason"] = ""
		updates["muted_until"] = 0
	}
	if len(updates) == 0 {
		return
	}
	if err := DBFromContext(ctx).WithContext(ctx).Model(&model.Account{}).
		Where("uid = ?", acc.UID).Updates(updates).Error; err != nil {
		clog.Warnf("account expiry clear uid=%d err=%v", acc.UID, err)
	}
}

// GetAccountByUID 按 uid 查账号；不返回密码。found=false 表示不存在。
func GetAccountByUID(uid int64) (*protocol.GmAccountView, bool, error) {
	return GetAccountByUIDContext(context.Background(), uid)
}

// GetAccountByUIDContext 按 uid 查账号。
func GetAccountByUIDContext(parent context.Context, uid int64) (*protocol.GmAccountView, bool, error) {
	if err := ensureDB(); err != nil {
		return nil, false, err
	}
	if uid < 1 {
		return nil, false, nil
	}
	ctx, cancel := opContext(parent)
	defer cancel()
	var acc model.Account
	err := DBFromContext(ctx).WithContext(ctx).Where("uid = ?", uid).First(&acc).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	applyAccountExpiry(ctx, &acc)
	return gmAccountView(acc), true, nil
}

// GetAccountByNickname 按昵称精确匹配查账号；不返回密码。
func GetAccountByNickname(nickname string) (*protocol.GmAccountView, bool, error) {
	return GetAccountByNicknameContext(context.Background(), nickname)
}

// GetAccountByNicknameContext 按昵称查账号。
func GetAccountByNicknameContext(parent context.Context, nickname string) (*protocol.GmAccountView, bool, error) {
	if err := ensureDB(); err != nil {
		return nil, false, err
	}
	nickname = strings.TrimSpace(nickname)
	if nickname == "" {
		return nil, false, nil
	}
	ctx, cancel := opContext(parent)
	defer cancel()
	var acc model.Account
	err := DBFromContext(ctx).WithContext(ctx).Where("nickname = ?", nickname).First(&acc).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	applyAccountExpiry(ctx, &acc)
	return gmAccountView(acc), true, nil
}

// GetPlayerByID 按 player_id 取角色，不校验归属；含已软删。found=false 表示不存在。
func GetPlayerByID(playerID int64) (*protocol.GmPlayerRecord, bool, error) {
	return GetPlayerByIDContext(context.Background(), playerID)
}

// GetPlayerByIDContext 按 player_id 取角色（含软删）。
func GetPlayerByIDContext(parent context.Context, playerID int64) (*protocol.GmPlayerRecord, bool, error) {
	if err := ensureDB(); err != nil {
		return nil, false, err
	}
	if playerID < 1 {
		return nil, false, nil
	}
	ctx, cancel := opContext(parent)
	defer cancel()
	var p model.Player
	err := DBFromContext(ctx).WithContext(ctx).Where("player_id = ?", playerID).First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return gmPlayerRecord(p), true, nil
}

// GetPlayersByName 按角色名精确匹配，含已软删。
func GetPlayersByName(name string) ([]*protocol.GmPlayerRecord, error) {
	return GetPlayersByNameContext(context.Background(), name)
}

// GetPlayersByNameContext 按角色名精确匹配。
func GetPlayersByNameContext(parent context.Context, name string) ([]*protocol.GmPlayerRecord, error) {
	if err := ensureDB(); err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, nil
	}
	ctx, cancel := opContext(parent)
	defer cancel()
	var players []model.Player
	if err := DBFromContext(ctx).WithContext(ctx).
		Where("name = ?", name).
		Order("player_id asc").
		Find(&players).Error; err != nil {
		return nil, err
	}
	out := make([]*protocol.GmPlayerRecord, 0, len(players))
	for i := range players {
		out = append(out, gmPlayerRecord(players[i]))
	}
	return out, nil
}

// ListPlayersByUIDAll 列出账号下全部角色（含软删），按创建时间升序。
func ListPlayersByUIDAll(uid int64) ([]*protocol.GmPlayerRecord, error) {
	return ListPlayersByUIDAllContext(context.Background(), uid)
}

// ListPlayersByUIDAllContext 列出账号下全部角色（含软删）。
func ListPlayersByUIDAllContext(parent context.Context, uid int64) ([]*protocol.GmPlayerRecord, error) {
	if err := ensureDB(); err != nil {
		return nil, err
	}
	if uid < 1 {
		return nil, nil
	}
	ctx, cancel := opContext(parent)
	defer cancel()
	var players []model.Player
	if err := DBFromContext(ctx).WithContext(ctx).
		Where("uid = ?", uid).
		Order("created_at asc, player_id asc").
		Find(&players).Error; err != nil {
		return nil, err
	}
	out := make([]*protocol.GmPlayerRecord, 0, len(players))
	for i := range players {
		out = append(out, gmPlayerRecord(players[i]))
	}
	return out, nil
}

// WriteGMOpLog 写入运营审计；失败只打日志，不返回错误以免拖垮业务。
func WriteGMOpLog(operator, action string, targetUID, targetPlayerID int64, detail string, resultCode int32) {
	if err := ensureDB(); err != nil {
		clog.Warnf("gm ops log skipped action=%s err=%v", action, err)
		return
	}
	if operator == "" {
		operator = "anonymous"
	}
	row := model.GMOpLog{
		Operator:       operator,
		Action:         action,
		TargetUID:      targetUID,
		TargetPlayerID: targetPlayerID,
		Detail:         detail,
		ResultCode:     resultCode,
	}
	if err := db.Create(&row).Error; err != nil {
		clog.Warnf("gm ops log insert fail action=%s err=%v", action, err)
	}
}

// IsAccountBanned 查询账号是否封禁；不存在视为未封禁。
func IsAccountBanned(uid int64) (bool, error) {
	return IsAccountBannedContext(context.Background(), uid)
}

// IsAccountBannedContext 查询账号是否封禁（含限时到期）。
func IsAccountBannedContext(parent context.Context, uid int64) (bool, error) {
	acc, err := loadAccountFlags(parent, uid)
	if err != nil || acc == nil {
		return false, err
	}
	return acc.Banned, nil
}

// IsAccountMuted 查询账号是否禁言；不存在视为未禁言。
func IsAccountMuted(uid int64) (bool, error) {
	return IsAccountMutedContext(context.Background(), uid)
}

// IsAccountMutedContext 查询账号是否禁言（含限时到期）。
func IsAccountMutedContext(parent context.Context, uid int64) (bool, error) {
	acc, err := loadAccountFlags(parent, uid)
	if err != nil || acc == nil {
		return false, err
	}
	return acc.Muted, nil
}

func loadAccountFlags(parent context.Context, uid int64) (*model.Account, error) {
	if err := ensureDB(); err != nil {
		return nil, err
	}
	if uid < 1 {
		return nil, nil
	}
	ctx, cancel := opContext(parent)
	defer cancel()
	var acc model.Account
	err := DBFromContext(ctx).WithContext(ctx).
		Select("uid", "banned", "ban_reason", "banned_until", "muted", "mute_reason", "muted_until").
		Where("uid = ?", uid).First(&acc).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	applyAccountExpiry(ctx, &acc)
	return &acc, nil
}

// SetAccountBanned 设置账号封禁状态（永久）；解封时清空原因与到期。
func SetAccountBanned(uid int64, banned bool, reason string) (bool, error) {
	return SetAccountBannedFor(uid, banned, reason, 0)
}

// SetAccountBannedFor 设置封禁；durationSeconds>0 为限时，0 为永久（banned=true）或清到期（解封）。
func SetAccountBannedFor(uid int64, banned bool, reason string, durationSeconds int64) (bool, error) {
	return SetAccountBannedForContext(context.Background(), uid, banned, reason, durationSeconds)
}

// SetAccountBannedForContext 设置账号封禁状态。
func SetAccountBannedForContext(parent context.Context, uid int64, banned bool, reason string, durationSeconds int64) (bool, error) {
	if err := ensureDB(); err != nil {
		return false, err
	}
	if uid < 1 {
		return false, nil
	}
	ctx, cancel := opContext(parent)
	defer cancel()
	reason = strings.TrimSpace(reason)
	var until int64
	if !banned {
		reason = ""
		until = 0
	} else if durationSeconds > 0 {
		until = gtime.UnixNow() + durationSeconds
	}
	res := DBFromContext(ctx).WithContext(ctx).Model(&model.Account{}).
		Where("uid = ?", uid).
		Updates(map[string]any{"banned": banned, "ban_reason": reason, "banned_until": until})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// SetAccountMuted 设置禁言；durationSeconds>0 为限时，0 为永久或解禁。
func SetAccountMuted(uid int64, muted bool, reason string, durationSeconds int64) (bool, error) {
	return SetAccountMutedContext(context.Background(), uid, muted, reason, durationSeconds)
}

// SetAccountMutedContext 设置账号禁言状态。
func SetAccountMutedContext(parent context.Context, uid int64, muted bool, reason string, durationSeconds int64) (bool, error) {
	if err := ensureDB(); err != nil {
		return false, err
	}
	if uid < 1 {
		return false, nil
	}
	ctx, cancel := opContext(parent)
	defer cancel()
	reason = strings.TrimSpace(reason)
	var until int64
	if !muted {
		reason = ""
		until = 0
	} else if durationSeconds > 0 {
		until = gtime.UnixNow() + durationSeconds
	}
	res := DBFromContext(ctx).WithContext(ctx).Model(&model.Account{}).
		Where("uid = ?", uid).
		Updates(map[string]any{"muted": muted, "mute_reason": reason, "muted_until": until})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}
