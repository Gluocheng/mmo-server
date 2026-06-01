package main

import (
	"flag"
	"os"

	clog "github.com/cherry-game/cherry/logger"
	"github.com/example/mmo-server/internal/gmapp"
)

func main() {
	var (
		httpAddr   = flag.String("http", ":9080", "GM HTTP 监听地址")
		natsAddr   = flag.String("nats", "nats://127.0.0.1:4222", "NATS 连接地址")
		natsPrefix = flag.String("prefix", "mmo", "NATS cluster prefix")
		gameNode   = flag.String("game", "10001", "目标 game 节点 ID")
	)
	flag.Parse()

	clog.Infof("gm: starting http=%s nats=%s prefix=%s game=%s",
		*httpAddr, *natsAddr, *natsPrefix, *gameNode)

	app := gmapp.New(*httpAddr, *natsAddr, *natsPrefix, *gameNode)
	if err := app.Run(); err != nil {
		clog.Errorf("gm: run error: %v", err)
		os.Exit(1)
	}
}
