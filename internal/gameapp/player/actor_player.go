package player

import (
	"context"
	"errors"
	"strconv"
	"strings"

	cstring "github.com/cherry-game/cherry/extend/string"
	clog "github.com/cherry-game/cherry/logger"
	"github.com/cherry-game/cherry/net/parser/pomelo"
	cproto "github.com/cherry-game/cherry/net/proto"
	gcruntime "github.com/example/mmo-server/gameconfig/pkg/runtime"
	"github.com/example/mmo-server/internal/code"
	"github.com/example/mmo-server/internal/gameapp/combat"
	"github.com/example/mmo-server/internal/gameapp/party"
	"github.com/example/mmo-server/internal/gameapp/world"
	"github.com/example/mmo-server/internal/gtime"
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
	p.Local().Register("switchScene", p.switchScene)
	p.Local().Register("scenes", p.scenes)
	p.Local().Register("delete", p.deletePlayer)
	p.Local().Register("move", p.move)
}

// sessionClose 断线时先写 60 秒宽限并卸下本机进场表，再离开场景。切图不走这里。
func (p *actorPlayer) sessionClose() {
	uid, _ := strconv.ParseInt(p.ActorID(), 10, 64)
	if err := persistence.BeginGrace(context.Background(), uid, gtime.Now().UnixMilli()); err != nil {
		// 不在队是断线常态，在线键已经删掉。其余错误（例如 Redis 不可用）单独警告。
		if err.Error() == "不在队伍中" {
			clog.Debugf("party begin grace uid=%d err=%v", uid, err)
		} else {
			clog.Warnf("party begin grace uid=%d err=%v", uid, err)
		}
	}
	party.Unbind(uid)
	world.Leave(uid)
	combat.Leave(uid)
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

	main, ok := gcruntime.MainScene()
	if !ok || main.MaxOnline < 1 {
		p.ResponseCode(session, code.SceneInvalid)
		return
	}
	view, c := world.ArriveMain(p, session.Uid, session.AgentPath, lineRule(main))
	if c != code.OK {
		p.ResponseCode(session, c)
		return
	}

	// 进房成功后再回写网关 session，避免主城满员时解锁后续玩法路由
	p.Call(session.ActorPath(), "setSession", &protocol.StringKeyValue{
		Key:   sessionkey.PlayerID,
		Value: cstring.ToString(info.PlayerId),
	})
	// 写完 session 再登记本机路径。回到队伍时只给本机已 Bind 的成员推 onParty。
	party.Bind(session.Uid, session.AgentPath)
	roster, back, err := persistence.OnEnter(context.Background(), session.Uid, p.Path().NodeID, gtime.Now().UnixMilli())
	if err != nil {
		clog.Warnf("party on enter uid=%d err=%v", session.Uid, err)
	} else if back {
		party.PushRoster(p, roster)
	}
	combat.Enter(session.Uid)
	nearby, ids := nearbyProto(view.Nearby)
	p.Response(session, &protocol.EnterGameResponse{
		SceneId: view.SceneID, Line: view.Line, X: view.X, Y: view.Y, Z: view.Z,
		Players: ids, Nearby: nearby,
	})
}

// switchScene 换到另一张地图。当前图留在当前线，不重置战斗，也不改组队在线状态。
func (p *actorPlayer) switchScene(session *cproto.Session, req *protocol.SceneSwitchRequest) {
	if !session.Contains(sessionkey.PlayerID) {
		p.ResponseCode(session, code.PlayerNotEntered)
		return
	}
	if req == nil || req.SceneId < 1 {
		p.ResponseCode(session, code.SceneInvalid)
		return
	}
	target, ok := gcruntime.Scene(req.SceneId)
	if !ok || target.MaxOnline < 1 {
		p.ResponseCode(session, code.SceneInvalid)
		return
	}
	leftCd := int32(0)
	if pose, in := world.PoseOf(session.Uid); in {
		if cur, found := gcruntime.Scene(pose.SceneID); found {
			leftCd = cur.SwitchCdMs
		}
	}
	view, c := world.SwitchMap(p, session.Uid, lineRule(target), leftCd, gtime.Now().UnixMilli())
	if c != code.OK {
		p.ResponseCode(session, c)
		return
	}
	if view.Changed {
		combat.Leave(session.Uid)
		combat.Enter(session.Uid)
	}
	nearby, _ := nearbyProto(view.Nearby)
	p.Response(session, &protocol.SceneSwitchResponse{
		SceneId: view.SceneID, Line: view.Line, X: view.X, Y: view.Y, Z: view.Z, Nearby: nearby,
	})
}

// scenes 返回配表里的地图和每一条线的当前人数。
func (p *actorPlayer) scenes(session *cproto.Session, _ *protocol.None) {
	if !session.Contains(sessionkey.PlayerID) {
		p.ResponseCode(session, code.PlayerNotEntered)
		return
	}
	rows := gcruntime.Scenes()
	rsp := &protocol.SceneListResponse{Scenes: make([]*protocol.SceneInfo, 0, len(rows))}
	for _, row := range rows {
		info := &protocol.SceneInfo{
			SceneId: row.ID, Name: row.Name, AllowCombat: row.AllowCombat,
			MaxOnline: row.MaxOnline, MaxLines: row.LineCount(),
			Lines: make([]*protocol.SceneLineCount, 0, row.LineCount()),
		}
		for line := int32(1); line <= row.LineCount(); line++ {
			info.Lines = append(info.Lines, &protocol.SceneLineCount{
				Line: line, Online: world.LineOnline(row.ID, line),
			})
		}
		rsp.Scenes = append(rsp.Scenes, info)
	}
	p.Response(session, rsp)
}

func lineRule(def gcruntime.SceneDef) world.LineRule {
	return world.LineRule{
		SceneID: def.ID, MaxOnline: def.MaxOnline, MaxLines: def.MaxLines,
		SpawnX: def.SpawnX, SpawnY: def.SpawnY, SpawnZ: def.SpawnZ, SwitchCdMs: def.SwitchCdMs,
	}
}

func nearbyProto(in []world.Nearby) ([]*protocol.SceneActor, []int64) {
	actors := make([]*protocol.SceneActor, 0, len(in))
	ids := make([]int64, 0, len(in))
	for _, n := range in {
		actors = append(actors, &protocol.SceneActor{
			Uid: n.UID, X: n.X, Y: n.Y, Z: n.Z,
			ActorType: n.ActorType, ConfigId: n.ConfigID,
		})
		// 进场响应的 players 只含玩家。怪物留在 nearby。
		if n.ActorType == world.ActorPlayer {
			ids = append(ids, n.UID)
		}
	}
	return actors, ids
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
