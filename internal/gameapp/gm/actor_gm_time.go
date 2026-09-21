package gm

import (
	"strconv"

	"github.com/cherry-game/cherry/net/parser/pomelo"
	"github.com/example/mmo-server/internal/code"
	"github.com/example/mmo-server/internal/gtime"
	"github.com/example/mmo-server/internal/persistence"
	"github.com/example/mmo-server/internal/protocol"
)

// actorGMTime GM 游戏时间偏置热更新。
type actorGMTime struct {
	pomelo.ActorBase
}

func (p *actorGMTime) OnInit() {
	p.Remote().Register("set", p.setCluster)
}

// setCluster 将本进程游戏时间偏置设为绝对值（秒，≥0）。
func (p *actorGMTime) setCluster(req *protocol.GmTimeSetRequest) (*protocol.GmTimeGetResponse, int32) {
	operator := ""
	var sec int64
	if req != nil {
		operator = req.Operator
		sec = req.BiasSeconds
	}
	if req == nil || sec < 0 {
		persistence.WriteGMOpLog(operator, persistence.GMActionTimeSet, 0, 0, "", code.GmBadRequest)
		return nil, code.GmBadRequest
	}
	gtime.SetBiasSeconds(sec)
	persistence.WriteGMOpLog(operator, persistence.GMActionTimeSet, 0, 0, strconv.FormatInt(sec, 10), code.OK)
	return currentTimeView(), code.OK
}

func currentTimeView() *protocol.GmTimeGetResponse {
	return &protocol.GmTimeGetResponse{
		BiasSeconds: gtime.BiasSeconds(),
		UnixNow:     gtime.UnixNow(),
		RealUnixNow: gtime.RealNow().Unix(),
	}
}
