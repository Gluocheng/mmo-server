package player

import (
	"strconv"
	"strings"

	cstring "github.com/cherry-game/cherry/extend/string"
	clog "github.com/cherry-game/cherry/logger"
	"github.com/cherry-game/cherry/net/parser/pomelo"
	cproto "github.com/cherry-game/cherry/net/proto"
	"github.com/example/mmo-server/internal/code"
	"github.com/example/mmo-server/internal/gameapp/world"
	"github.com/example/mmo-server/internal/persistence"
	"github.com/example/mmo-server/internal/protocol"
	"github.com/example/mmo-server/internal/sessionkey"
	"google.golang.org/protobuf/types/known/emptypb"
)

type actorPlayer struct {
	pomelo.ActorBase
}

func (p *actorPlayer) OnInit() {
	p.Remote().Register("sessionClose", p.sessionClose)
	p.Local().Register("select", p.selectPlayer)
	p.Local().Register("create", p.createPlayer)
	p.Local().Register("enter", p.enter)
	p.Local().Register("move", p.move)
}

func (p *actorPlayer) sessionClose() {
	uid, _ := strconv.ParseInt(p.ActorID(), 10, 64)
	world.Leave(uid)
	p.Exit()
	clog.Debugf("player actor exit uid=%d path=%s", uid, p.PathString())
}

func (p *actorPlayer) selectPlayer(session *cproto.Session, _ *protocol.None) {
	rsp := &protocol.PlayerSelectResponse{}
	info, ok, err := persistence.GetPlayerByUID(session.Uid)
	if err != nil {
		clog.Warnf("select player fail uid=%d err=%v", session.Uid, err)
		p.ResponseCode(session, code.PlayerNotFound)
		return
	}
	if ok {
		pi := info
		rsp.List = append(rsp.List, &pi)
	}
	p.Response(session, rsp)
}

func (p *actorPlayer) createPlayer(session *cproto.Session, req *protocol.PlayerCreateRequest) {
	if req == nil || strings.TrimSpace(req.Name) == "" {
		p.ResponseCode(session, code.PlayerCreateFail)
		return
	}
	info, created, err := persistence.CreatePlayer(session.Uid, req.Name)
	if err != nil {
		clog.Warnf("create player fail uid=%d err=%v", session.Uid, err)
		p.ResponseCode(session, code.PlayerCreateFail)
		return
	}
	if !created {
		p.ResponseCode(session, code.PlayerCreateFail)
		return
	}
	pi := info
	p.Response(session, &protocol.PlayerCreateResponse{Player: &pi})
}

func (p *actorPlayer) enter(session *cproto.Session, req *protocol.EnterGameRequest) {
	if session.Uid < 1 {
		p.ResponseCode(session, code.NotLoggedIn)
		return
	}
	info, ok, err := persistence.GetPlayerByUID(session.Uid)
	if err != nil {
		clog.Warnf("load player fail uid=%d err=%v", session.Uid, err)
		p.ResponseCode(session, code.PlayerNotFound)
		return
	}
	if !ok {
		p.ResponseCode(session, code.PlayerNotFound)
		return
	}
	if req != nil && req.PlayerId > 0 && req.PlayerId != info.PlayerId {
		p.ResponseCode(session, code.PlayerNotFound)
		return
	}

	// 回写网关 session，启用后续 gameplay 路由
	p.Call(session.ActorPath(), "setSession", &protocol.StringKeyValue{
		Key:   sessionkey.PlayerID,
		Value: cstring.ToString(info.PlayerId),
	})

	sceneID := world.DefaultSceneID
	if req != nil && req.SceneId > 0 {
		sceneID = req.SceneId
	}
	_ = sceneID // 当前仅单场景
	all := world.Enter(session.Uid, session.AgentPath)
	p.Response(session, &protocol.EnterGameResponse{SceneId: world.DefaultSceneID, Players: all})
}

func (p *actorPlayer) move(session *cproto.Session, req *protocol.MoveRequest) {
	if !session.Contains(sessionkey.PlayerID) {
		p.ResponseCode(session, code.PlayerNotEntered)
		return
	}
	if req == nil {
		p.ResponseCode(session, code.EnterSceneFail)
		return
	}
	b := &protocol.MoveBroadcast{
		Uid: session.Uid,
		X:   req.X,
		Y:   req.Y,
		Z:   req.Z,
	}
	world.BroadcastMove(p, session.Uid, b)
	p.Response(session, &emptypb.Empty{})
}
