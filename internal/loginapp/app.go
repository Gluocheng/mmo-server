package loginapp

import (
	"github.com/cherry-game/cherry"
	cserializer "github.com/cherry-game/cherry/net/serializer"
	"github.com/example/mmo-server/internal/loginapp/actor"
)

// Run 启动登录节点（仅处理帐号/访客登录，演示用内存 UID）
func Run(profileFilePath, nodeID string) {
	app := cherry.Configure(profileFilePath, nodeID, false, cherry.Cluster)
	app.SetSerializer(cserializer.NewProtobuf())
	app.AddActors(&actor.ActorSession{})
	app.Startup()
}
