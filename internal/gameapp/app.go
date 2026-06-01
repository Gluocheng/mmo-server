package gameapp

import (
	"context"

	"github.com/cherry-game/cherry"
	cherrySnowflake "github.com/cherry-game/cherry/extend/snowflake"
	cstring "github.com/cherry-game/cherry/extend/string"
	cherryUtils "github.com/cherry-game/cherry/extend/utils"
	cserializer "github.com/cherry-game/cherry/net/serializer"
	gcruntime "github.com/example/mmo-server/gameconfig/pkg/runtime"
	"github.com/example/mmo-server/internal/gameapp/bag"
	"github.com/example/mmo-server/internal/gameapp/chat"
	"github.com/example/mmo-server/internal/gameapp/config"
	"github.com/example/mmo-server/internal/gameapp/player"
	"github.com/example/mmo-server/internal/persistence"
)

// Run 启动游戏节点
func Run(profileFilePath, nodeID string) {
	if !cherryUtils.IsNumeric(nodeID) {
		panic("game node id must be numeric, e.g. 10001")
	}
	serverID, _ := cstring.ToInt64(nodeID)
	cherrySnowflake.SetDefaultNode(serverID)

	app := cherry.Configure(profileFilePath, nodeID, false, cherry.Cluster)
	app.SetSerializer(cserializer.NewProtobuf())
	mustLoadGameConfig()
	app.AddActors(&player.ActorPlayers{})
	app.AddActors(&chat.ActorChats{})
	app.AddActors(&bag.ActorBags{})
	app.AddActors(&config.ActorConfig{})
	app.Startup()
}

// mustLoadGameConfig 初始化持久化并从 MySQL 加载策划配置。
func mustLoadGameConfig() {
	if err := persistence.Init(); err != nil {
		panic("gameconfig: persistence init: " + err.Error())
	}
	db, err := persistence.DB()
	if err != nil {
		panic("gameconfig: get db: " + err.Error())
	}
	gcruntime.MustLoad(context.Background(), db)
}
