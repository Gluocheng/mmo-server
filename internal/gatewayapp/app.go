package gatewayapp

import (
	"github.com/cherry-game/cherry"
	cconnector "github.com/cherry-game/cherry/net/connector"
	"github.com/cherry-game/cherry/net/parser/pomelo"
	cserializer "github.com/cherry-game/cherry/net/serializer"
	"github.com/example/mmo-server/internal/gatewayapp/actor"
)

// Run 启动网关节点（WebSocket + Pomelo 协议）
func Run(profileFilePath, nodeID string) {
	app := cherry.Configure(profileFilePath, nodeID, true, cherry.Cluster)
	app.SetSerializer(cserializer.NewProtobuf())

	agentActor := pomelo.NewActor("user")
	agentActor.AddConnector(cconnector.NewWS(app.Address()))
	agentActor.SetOnNewAgent(func(newAgent *pomelo.Agent) {
		child := &actor.AgentActor{}
		newAgent.AddOnClose(child.OnSessionClose)
		agentActor.Child().Create(newAgent.SID(), child)
	})
	agentActor.SetOnDataRoute(actor.OnPomeloDataRoute)
	app.SetNetParser(agentActor)

	app.Startup()
}
