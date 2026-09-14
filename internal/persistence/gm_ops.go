package persistence

import (
	"context"
	"errors"
	"strings"

	clog "github.com/cherry-game/cherry/logger"
	"github.com/example/mmo-server/internal/persistence/model"
	"github.com/example/mmo-server/internal/protocol"
	"gorm.io/gorm"
)

const (
	GMActionReload = "config.reload"
	GMActionGrant  = "bag.grant"
	GMActionKick   = "player.kick"
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
