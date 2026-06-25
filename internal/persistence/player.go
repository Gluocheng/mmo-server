package persistence

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/example/mmo-server/internal/protocol"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
)

// 多角色相关错误。
var (
	// ErrPlayerLimitExceeded 账号未删除角色数已达上限。
	ErrPlayerLimitExceeded = errors.New("player limit exceeded")
	// ErrPlayerNameTaken 全服未删除角色名已被占用。
	ErrPlayerNameTaken = errors.New("player name taken")
	// ErrPlayerNotFound 角色 player_id 不存在或不属于该账号。
	ErrPlayerNotFound = errors.New("player not found")
	// ErrPlayerDeleted 角色已被软删除。
	ErrPlayerDeleted = errors.New("player deleted")
)

func playerInfoFromModel(model Player) *protocol.PlayerInfo {
	return &protocol.PlayerInfo{
		PlayerId: model.PlayerID,
		Name:     model.Name,
	}
}

// countPlayersByUIDInTx 统计该账号下未删除角色数，用于上限校验。
func countPlayersByUIDInTx(ctx context.Context, uid int64) (int64, error) {
	var n int64
	err := DBFromContext(ctx).WithContext(ctx).
		Model(&Player{}).
		Where("uid = ? AND deleted_at IS NULL", uid).
		Count(&n).Error
	return n, err
}

// ListPlayersByUIDContext 返回该账号下未删除角色列表，按创建时间升序。
func ListPlayersByUIDContext(parent context.Context, uid int64) ([]*protocol.PlayerInfo, error) {
	if err := ensureDB(); err != nil {
		return nil, err
	}
	if uid < 1 {
		return nil, nil
	}

	ctx, cancel := opContext(parent)
	defer cancel()

	var models []Player
	if err := DBFromContext(ctx).WithContext(ctx).
		Where("uid = ? AND deleted_at IS NULL", uid).
		Order("created_at asc, player_id asc").
		Find(&models).Error; err != nil {
		return nil, err
	}
	out := make([]*protocol.PlayerInfo, 0, len(models))
	for i := range models {
		out = append(out, playerInfoFromModel(models[i]))
	}
	return out, nil
}

// ListPlayersByUID 是 ListPlayersByUIDContext 的便捷入口。
func ListPlayersByUID(uid int64) ([]*protocol.PlayerInfo, error) {
	return ListPlayersByUIDContext(context.Background(), uid)
}

// GetPlayerByPlayerIDContext 按 player_id 取角色，并校验归属与未删除；
// found 为 false 表示不存在、不属于该账号或已删除。
func GetPlayerByPlayerIDContext(parent context.Context, uid, playerID int64) (*protocol.PlayerInfo, bool, error) {
	if err := ensureDB(); err != nil {
		return nil, false, err
	}
	if uid < 1 || playerID < 1 {
		return nil, false, nil
	}

	ctx, cancel := opContext(parent)
	defer cancel()

	var model Player
	err := DBFromContext(ctx).WithContext(ctx).
		Where("player_id = ? AND uid = ?", playerID, uid).
		First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if model.DeletedAt != nil {
		return nil, false, nil
	}
	return proto.Clone(playerInfoFromModel(model)).(*protocol.PlayerInfo), true, nil
}

// GetPlayerByPlayerID 是 GetPlayerByPlayerIDContext 的便捷入口。
func GetPlayerByPlayerID(uid, playerID int64) (*protocol.PlayerInfo, bool, error) {
	return GetPlayerByPlayerIDContext(context.Background(), uid, playerID)
}

// createPlayerInTx 在事务内创建新角色，受 MaxCharacters 上限约束；
// 不再像一账号一角色时直接返回旧角色。名称全服未删除唯一由查询保证。
func createPlayerInTx(ctx context.Context, uid int64, name string) (*protocol.PlayerInfo, bool, error) {
	name = strings.TrimSpace(name)
	if uid < 1 || name == "" {
		return nil, false, nil
	}

	// 上限校验：软删角色不释放配额。
	count, err := countPlayersByUIDInTx(ctx, uid)
	if err != nil {
		return nil, false, err
	}
	if count >= int64(MaxCharacters()) {
		return nil, false, ErrPlayerLimitExceeded
	}

	// 全服未删除角色名唯一。
	var dup Player
	dupErr := DBFromContext(ctx).WithContext(ctx).
		Where("name = ? AND deleted_at IS NULL", name).
		First(&dup).Error
	if dupErr == nil {
		return nil, false, ErrPlayerNameTaken
	} else if !errors.Is(dupErr, gorm.ErrRecordNotFound) {
		return nil, false, dupErr
	}

	playerID, err := nextPlayerIDInTx(ctx)
	if err != nil {
		return nil, false, err
	}

	model := Player{
		PlayerID: playerID,
		UID:      uid,
		Name:     name,
	}
	if err := DBFromContext(ctx).WithContext(ctx).Create(&model).Error; err != nil {
		return nil, false, err
	}
	return playerInfoFromModel(model), true, nil
}

// deletePlayerInTx 软删除角色，校验归属；已删除返回 ErrPlayerDeleted。
func deletePlayerInTx(ctx context.Context, uid, playerID int64) error {
	if uid < 1 || playerID < 1 {
		return ErrPlayerNotFound
	}

	var model Player
	err := DBFromContext(ctx).WithContext(ctx).
		Where("player_id = ? AND uid = ?", playerID, uid).
		First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrPlayerNotFound
	}
	if err != nil {
		return err
	}
	if model.DeletedAt != nil {
		return ErrPlayerDeleted
	}

	now := time.Now()
	return DBFromContext(ctx).WithContext(ctx).
		Model(&model).
		Update("deleted_at", now).Error
}
