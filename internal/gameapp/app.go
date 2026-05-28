package gameapp

import (
	"github.com/cherry-game/cherry"
	cstring "github.com/cherry-game/cherry/extend/string"
	cherryUtils "github.com/cherry-game/cherry/extend/utils"
	cherrySnowflake "github.com/cherry-game/cherry/extend/snowflake"
	cserializer "github.com/cherry-game/cherry/net/serializer"
	"github.com/example/mmo-server/internal/gameapp/chat"
	"github.com/example/mmo-server/internal/gameapp/player"
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
	app.AddActors(&player.ActorPlayers{})
	app.AddActors(&chat.ActorChats{})
	app.Startup()
}
