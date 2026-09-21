package main

import (
	"flag"
	"os"
	"strings"

	clog "github.com/cherry-game/cherry/logger"
	"github.com/example/mmo-server/internal/gmapp"
)

func main() {
	var (
		httpAddr   = flag.String("http", ":9080", "GM HTTP 监听地址")
		natsAddr   = flag.String("nats", "nats://127.0.0.1:4222", "NATS 连接地址")
		natsPrefix = flag.String("prefix", "mmo", "NATS cluster prefix")
		gameNode   = flag.String("game", "10001", "目标 game 节点 ID")
		loginNode  = flag.String("login", "login-1", "目标 login 节点 ID（调游戏时间热更新）")
		pathFlag   = flag.String("path", "configs/mmo-cluster.json", "profile json path")
		tokenFlag  = flag.String("token", "", "GM HTTP 共享密钥，空则读环境变量 GM_HTTP_TOKEN")
	)
	flag.Parse()

	token := strings.TrimSpace(*tokenFlag)
	if token == "" {
		token = strings.TrimSpace(os.Getenv("GM_HTTP_TOKEN"))
	}
	bootUser := strings.TrimSpace(os.Getenv("GM_BOOTSTRAP_USER"))
	bootPass := strings.TrimSpace(os.Getenv("GM_BOOTSTRAP_PASSWORD"))

	clog.Infof("gm: starting http=%s nats=%s prefix=%s game=%s login=%s path=%s token=%v",
		*httpAddr, *natsAddr, *natsPrefix, *gameNode, *loginNode, *pathFlag, token != "")

	app := gmapp.New(*httpAddr, *natsAddr, *natsPrefix, *gameNode, token)
	app.SetProfile(*pathFlag, bootUser, bootPass)
	app.SetLoginNode(*loginNode)
	if err := app.Run(); err != nil {
		clog.Errorf("gm: run error: %v", err)
		os.Exit(1)
	}
}
