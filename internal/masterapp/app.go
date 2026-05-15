package masterapp

import (
	"github.com/cherry-game/cherry"
	cserializer "github.com/cherry-game/cherry/net/serializer"
)

// Run 启动 discovery master 节点（仅集群注册发现，无业务 Actor）
// 须先于 gate/login/game 启动，且 profile 中 cluster.discovery.mode 为 nats、cluster.nats.master_node_id 与本节点 node_id 一致。
func Run(profileFilePath, nodeID string) {
	app := cherry.Configure(profileFilePath, nodeID, false, cherry.Cluster)
	app.SetSerializer(cserializer.NewProtobuf())
	app.Startup()
}
