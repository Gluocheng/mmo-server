package config

import (
	"github.com/cherry-game/cherry/net/parser/pomelo"
)

// actorConfig 配置子 Actor。配表热更不对 Pomelo 客户端暴露，只走 GM HTTP。
type actorConfig struct {
	pomelo.ActorBase
}
