package loginapp

import (
	"github.com/cherry-game/cherry"
	cserializer "github.com/cherry-game/cherry/net/serializer"
	"github.com/example/mmo-server/internal/loginapp/actor"
	"github.com/example/mmo-server/internal/nodecfg"
	"github.com/example/mmo-server/internal/persistence"
)

// Run 启动登录节点，并用 nodeID 初始化账号 UID 生成器。
func Run(profileFilePath, nodeID string) {
	if err := persistence.ConfigureIDNodeFromString(nodeID); err != nil {
		panic("login: configure id node: " + err.Error())
	}

	app := cherry.Configure(profileFilePath, nodeID, false, cherry.Cluster)
	nodecfg.ApplyCallTimeout(app)
	app.SetSerializer(cserializer.NewProtobuf())
	app.AddActors(&actor.ActorSession{})
	app.Startup()
}
