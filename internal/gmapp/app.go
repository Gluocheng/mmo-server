package gmapp

import (
	"fmt"
	"net/http"
	"strings"

	clog "github.com/cherry-game/cherry/logger"
	cprofile "github.com/cherry-game/cherry/profile"
	"github.com/example/mmo-server/internal/persistence"
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
	loginNodeID   string
	token         string
	profilePath   string
	bootstrapUser string
	bootstrapPass string
	remoteSubject string // NATS subject: cherry-{prefix}.remote.game.{gameNodeID}
	sourcePath    string // ClusterPacket: {gmNodeID}.gm.config（health 展示）
	targetPath    string // ClusterPacket: {gameNodeID}.gm.config（health 展示）
}

// New 创建 GM 应用实例。token 为空时除公开接口外须登录会话。
func New(httpAddr, natsAddr, natsPrefix, gameNodeID, token string) *App {
	return &App{
		httpAddr:    httpAddr,
		natsAddr:    natsAddr,
		natsPrefix:  natsPrefix,
		gameNodeID:  gameNodeID,
		loginNodeID: "login-1",
		token:       token,
	}
}

// SetProfile 设置 cluster profile 路径与空表种子管理员。
func (a *App) SetProfile(path, bootstrapUser, bootstrapPass string) {
	a.profilePath = path
	a.bootstrapUser = bootstrapUser
	a.bootstrapPass = bootstrapPass
}

// SetLoginNode 设置热更新游戏时间时通知的 login 节点 ID。
func (a *App) SetLoginNode(id string) {
	id = strings.TrimSpace(id)
	if id != "" {
		a.loginNodeID = id
	}
}

// Run 启动 GM 进程：加载 profile、连接 MySQL/Redis 与 NATS，再启动 HTTP。
func (a *App) Run() error {
	if a.profilePath == "" {
		a.profilePath = "configs/mmo-cluster.json"
	}
	if _, err := cprofile.Init(a.profilePath, gmNodeID); err != nil {
		return fmt.Errorf("gm profile: %w", err)
	}
	if err := persistence.Init(); err != nil {
		return fmt.Errorf("gm persistence: %w", err)
	}
	if err := persistence.EnsureGMBootstrap(a.bootstrapUser, a.bootstrapPass); err != nil {
		return fmt.Errorf("gm bootstrap: %w", err)
	}

	nc, err := nats.Connect(a.natsAddr)
	if err != nil {
		return fmt.Errorf("gm nats connect: %w", err)
	}
	a.natsConn = nc
	defer nc.Close()
	clog.Infof("gm: nats connected to %s", nc.ConnectedUrl())

	a.remoteSubject = fmt.Sprintf("cherry-%s.remote.game.%s", a.natsPrefix, a.gameNodeID)
	a.sourcePath = fmt.Sprintf("%s.gm.config", gmNodeID)
	a.targetPath = fmt.Sprintf("%s.gm.config", a.gameNodeID)

	mux := http.NewServeMux()
	a.registerRoutes(mux)

	clog.Infof("gm: http listening on %s", a.httpAddr)
	return http.ListenAndServe(a.httpAddr, mux)
}
