package persistence

import (
	"context"
	"errors"
	"hash/crc32"
	"sync"

	cherrySnowflake "github.com/cherry-game/cherry/extend/snowflake"
	"github.com/example/mmo-server/internal/persistence/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// playerIDInitialValue 是角色短数字 ID 的默认起始值，避开历史小自增 ID。
const playerIDInitialValue int64 = 100001

// playerIDBatchSize 是一次从数据库领走的角色编号个数。
// 创角事务不再逐个锁序列行，避免大量同时创角在 2 秒操作超时内排不完。
const playerIDBatchSize int64 = 256

// sequencePlayerID 是 id_sequences 中角色 ID 序列的名称。
const sequencePlayerID = "player_id"

var (
	uidNodeMu sync.Mutex
	uidNode   *cherrySnowflake.Node

	playerIDMu   sync.Mutex
	playerIDNext int64
	playerIDEnd  int64
)

// ConfigureIDNodeFromString 按节点字符串初始化 UID 生成器；多登录节点必须使用不同 nodeID。
func ConfigureIDNodeFromString(seed string) error {
	nodeValue := int64(crc32.ChecksumIEEE([]byte(seed))) % (1 << cherrySnowflake.NodeBits)
	node, err := cherrySnowflake.NewNode(nodeValue)
	if err != nil {
		return err
	}

	uidNodeMu.Lock()
	uidNode = node
	uidNodeMu.Unlock()
	return nil
}

func nextUID() (int64, error) {
	uidNodeMu.Lock()
	node := uidNode
	uidNodeMu.Unlock()

	if node == nil {
		if err := ConfigureIDNodeFromString("persistence-default"); err != nil {
			return 0, err
		}
		uidNodeMu.Lock()
		node = uidNode
		uidNodeMu.Unlock()
	}
	return node.Generate().Int64(), nil
}

// reservePlayerID 从本进程缓存领一个角色编号。缓存空时另开短事务一次领一段。
// 编号在领走时就提交，创角失败或回滚不会把编号退回。
func reservePlayerID(ctx context.Context) (int64, error) {
	playerIDMu.Lock()
	defer playerIDMu.Unlock()
	if playerIDNext < playerIDEnd {
		id := playerIDNext
		playerIDNext++
		return id, nil
	}
	start, err := reservePlayerIDBlock(ctx, playerIDBatchSize)
	if err != nil {
		return 0, err
	}
	playerIDNext = start + 1
	playerIDEnd = start + playerIDBatchSize
	return start, nil
}

func reservePlayerIDBlock(ctx context.Context, n int64) (int64, error) {
	if err := ensureDB(); err != nil {
		return 0, err
	}
	var start int64
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var seq model.IDSequence
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("name = ?", sequencePlayerID).
			First(&seq).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			seq = model.IDSequence{Name: sequencePlayerID, NextValue: playerIDInitialValue}
			if err := tx.Create(&seq).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		start = seq.NextValue
		if start < playerIDInitialValue {
			start = playerIDInitialValue
		}
		return tx.Model(&model.IDSequence{}).
			Where("name = ?", sequencePlayerID).
			Update("next_value", start+n).Error
	})
	return start, err
}

func resetPlayerIDBatch() {
	playerIDMu.Lock()
	playerIDNext = 0
	playerIDEnd = 0
	playerIDMu.Unlock()
}

func nextPlayerIDInTx(ctx context.Context) (int64, error) {
	db := DBFromContext(ctx).WithContext(ctx)
	var seq model.IDSequence
	err := db.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("name = ?", sequencePlayerID).
		First(&seq).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		seq = model.IDSequence{Name: sequencePlayerID, NextValue: playerIDInitialValue}
		if err := db.Create(&seq).Error; err != nil {
			return 0, err
		}
	} else if err != nil {
		return 0, err
	}

	playerID := seq.NextValue
	if playerID < playerIDInitialValue {
		playerID = playerIDInitialValue
	}
	if err := db.Model(&model.IDSequence{}).
		Where("name = ?", sequencePlayerID).
		Update("next_value", playerID+1).Error; err != nil {
		return 0, err
	}
	return playerID, nil
}
