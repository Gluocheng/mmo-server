package gmapp

import (
	"fmt"
	"net/http"

	clog "github.com/cherry-game/cherry/logger"
	"github.com/nats-io/nats.go"
)

// GM 节点信息（硬编码，后续可从配置读取）。
const (
	gmNodeID   = "gm-1"
	gmNodeType = "gm"
)

// App GM 独立进程：HTTP 管理接口，通过 NATS 与 game 节点通信。
type App struct {
	natsConn      *nats.Conn
	httpAddr      string
	natsAddr      string
	natsPrefix    string
	gameNodeID    string
	remoteSubject string // NATS subject: cherry-{prefix}.remote.game.{gameNodeID}
	sourcePath    string // ClusterPacket: {gmNodeID}.gm.config
	targetPath    string // ClusterPacket: {gameNodeID}.gm.config
}

// New 创建 GM 应用实例。
func New(httpAddr, natsAddr, natsPrefix, gameNodeID string) *App {
	return &App{
		httpAddr:   httpAddr,
		natsAddr:   natsAddr,
		natsPrefix: natsPrefix,
		gameNodeID: gameNodeID,
	}
}

// Run 启动 GM 进程：连接 NATS 并启动 HTTP 服务。
func (a *App) Run() error {
	nc, err := nats.Connect(a.natsAddr)
	if err != nil {
		return fmt.Errorf("gm nats connect: %w", err)
	}
	a.natsConn = nc
	clog.Infof("gm: nats connected to %s", nc.ConnectedUrl())

	// 初始化 NATS 路由信息
	a.remoteSubject = fmt.Sprintf("cherry-%s.remote.game.%s", a.natsPrefix, a.gameNodeID)
	a.sourcePath = fmt.Sprintf("%s.gm.config", gmNodeID)
	a.targetPath = fmt.Sprintf("%s.gm.config", a.gameNodeID)

	mux := http.NewServeMux()
	a.registerRoutes(mux)

	clog.Infof("gm: http listening on %s", a.httpAddr)
	return http.ListenAndServe(a.httpAddr, mux)
}
