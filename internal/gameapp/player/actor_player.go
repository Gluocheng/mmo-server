package player

import (
	"errors"
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
	"google.golang.org/protobuf/proto"
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
	p.Local().Register("delete", p.deletePlayer)
	p.Local().Register("move", p.move)
}

func (p *actorPlayer) sessionClose() {
	uid, _ := strconv.ParseInt(p.ActorID(), 10, 64)
	world.Leave(uid)
	p.Exit()
	clog.Debugf("player actor exit uid=%d path=%s", uid, p.PathString())
}

// selectPlayer 返回当前账号下全部未删除角色列表（一账号多角）。
func (p *actorPlayer) selectPlayer(session *cproto.Session, _ *protocol.None) {
	rsp := &protocol.PlayerSelectResponse{}
	players, err := persistence.ListPlayersByUID(session.Uid)
	if err != nil {
		clog.Warnf("select player fail uid=%d err=%v", session.Uid, err)
		p.ResponseCode(session, code.PlayerNotFound)
		return
	}
	for i := range players {
		rsp.List = append(rsp.List, proto.Clone(players[i]).(*protocol.PlayerInfo))
	}
	p.Response(session, rsp)
}

// createPlayer 创建新角色，受配置上限与全服未删除重名约束。
func (p *actorPlayer) createPlayer(session *cproto.Session, req *protocol.PlayerCreateRequest) {
	if req == nil || strings.TrimSpace(req.Name) == "" {
		p.ResponseCode(session, code.PlayerCreateFail)
		return
	}
	info, created, err := persistence.CreatePlayerForUID(session.Uid, req.Name)
	if err != nil {
		switch {
		case errors.Is(err, persistence.ErrPlayerLimitExceeded):
			p.ResponseCode(session, code.PlayerLimitExceeded)
		case errors.Is(err, persistence.ErrPlayerNameTaken):
			p.ResponseCode(session, code.PlayerCreateFail)
		default:
			clog.Warnf("create player fail uid=%d err=%v", session.Uid, err)
			p.ResponseCode(session, code.PlayerCreateFail)
		}
		return
	}
	if !created || info == nil {
		p.ResponseCode(session, code.PlayerCreateFail)
		return
	}
	p.Response(session, &protocol.PlayerCreateResponse{Player: proto.Clone(info).(*protocol.PlayerInfo)})
}

// enter 进场，player_id 必填且必须属于当前账号、未删除。
func (p *actorPlayer) enter(session *cproto.Session, req *protocol.EnterGameRequest) {
	if session.Uid < 1 {
		p.ResponseCode(session, code.NotLoggedIn)
		return
	}
	if req == nil || req.PlayerId < 1 {
		p.ResponseCode(session, code.PlayerNotFound)
		return
	}
	info, ok, err := persistence.GetPlayerByPlayerID(session.Uid, req.PlayerId)
	if err != nil {
		clog.Warnf("load player fail uid=%d player_id=%d err=%v", session.Uid, req.PlayerId, err)
		p.ResponseCode(session, code.PlayerNotFound)
		return
	}
	if !ok || info == nil {
		p.ResponseCode(session, code.PlayerNotFound)
		return
	}

	// 回写网关 session，启用后续 gameplay 路由
	p.Call(session.ActorPath(), "setSession", &protocol.StringKeyValue{
		Key:   sessionkey.PlayerID,
		Value: cstring.ToString(info.PlayerId),
	})

	sceneID := world.DefaultSceneID
	if req.SceneId > 0 {
		sceneID = req.SceneId
	}
	all := world.Enter(session.Uid, session.AgentPath, sceneID)
	p.Response(session, &protocol.EnterGameResponse{SceneId: sceneID, Players: all})
}

// deletePlayer 软删除角色，校验归属；已删除返回 40027。
func (p *actorPlayer) deletePlayer(session *cproto.Session, req *protocol.PlayerDeleteRequest) {
	if req == nil || req.PlayerId < 1 {
		p.ResponseCode(session, code.PlayerNotFound)
		return
	}
	err := persistence.DeletePlayer(session.Uid, req.PlayerId)
	if err != nil {
		switch {
		case errors.Is(err, persistence.ErrPlayerDeleted):
			p.ResponseCode(session, code.PlayerDeleted)
		case errors.Is(err, persistence.ErrPlayerNotFound):
			p.ResponseCode(session, code.PlayerNotFound)
		default:
			clog.Warnf("delete player fail uid=%d player_id=%d err=%v", session.Uid, req.PlayerId, err)
			p.ResponseCode(session, code.PlayerNotFound)
		}
		return
	}
	p.Response(session, &emptypb.Empty{})
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
