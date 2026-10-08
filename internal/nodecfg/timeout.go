package nodecfg

import (
	"time"

	cfacade "github.com/cherry-game/cherry/facade"
	cprofile "github.com/cherry-game/cherry/profile"
)

// ApplyCallTimeout 让节点间 CallWait 使用 profile 里的 cluster.nats.request_timeout（秒）。
// Actor 系统默认只等 3 秒，大量注册会在密码哈希完成前被当成 RPC 失败。
func ApplyCallTimeout(app cfacade.IApplication) {
	if app == nil || app.ActorSystem() == nil {
		return
	}
	seconds := cprofile.GetConfig("cluster").GetConfig("nats").GetDuration("request_timeout", 3)
	if seconds < 1 {
		seconds = 3
	}
	app.ActorSystem().SetCallTimeout(seconds * time.Second)
}
