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

// sequencePlayerID 是 id_sequences 中角色 ID 序列的名称。
const sequencePlayerID = "player_id"

var (
	uidNodeMu sync.Mutex
	uidNode   *cherrySnowflake.Node
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
