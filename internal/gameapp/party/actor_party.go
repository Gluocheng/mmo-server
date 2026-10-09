package party

import (
	"context"

	cherrySnowflake "github.com/cherry-game/cherry/extend/snowflake"
	cfacade "github.com/cherry-game/cherry/facade"
	clog "github.com/cherry-game/cherry/logger"
	"github.com/cherry-game/cherry/net/parser/pomelo"
	cproto "github.com/cherry-game/cherry/net/proto"
	"github.com/example/mmo-server/internal/code"
	"github.com/example/mmo-server/internal/gtime"
	"github.com/example/mmo-server/internal/persistence"
	"github.com/example/mmo-server/internal/protocol"
	"github.com/example/mmo-server/internal/sessionkey"
	"google.golang.org/protobuf/types/known/emptypb"
)

// 邀请从发出起 30 秒内有效，与 Redis 邀请 TTL 一致。expireAt 用游戏时间毫秒。
const inviteValidMS int64 = 30_000

// actorParty 处理单个 uid 的组队请求。未进场返回 40009。
type actorParty struct {
	pomelo.ActorBase
}

// OnInit 注册 create、invite、answer、leave、kick、state。
func (p *actorParty) OnInit() {
	p.Local().Register("create", p.create)
	p.Local().Register("invite", p.invite)
	p.Local().Register("answer", p.answer)
	p.Local().Register("leave", p.leave)
	p.Local().Register("kick", p.kick)
	p.Local().Register("state", p.state)
}

func (p *actorParty) nodeID() string {
	return p.Path().NodeID
}

func (p *actorParty) requireEntered(session *cproto.Session) bool {
	if !session.Contains(sessionkey.PlayerID) {
		p.ResponseCode(session, code.PlayerNotEntered)
		return false
	}
	return true
}

func (p *actorParty) reject(session *cproto.Session, op string, c int32) {
	if c == code.LoginFail {
		clog.Warnf("party %s uid=%d code=%d", op, session.Uid, c)
	}
	p.ResponseCode(session, c)
}

func (p *actorParty) create(session *cproto.Session, _ *protocol.None) {
	if !p.requireEntered(session) {
		return
	}
	now := gtime.Now().UnixMilli()
	st, c := persistence.CreateParty(context.Background(), session.Uid, cherrySnowflake.NextID(), p.nodeID(), now)
	if c != code.OK {
		p.reject(session, "create", c)
		return
	}
	view := protoState(st)
	publish(p, &protocol.PartyDeliver{Uids: memberUIDs(st), State: view})
	p.Response(session, view)
}

func (p *actorParty) invite(session *cproto.Session, req *protocol.PartyInviteRequest) {
	if !p.requireEntered(session) {
		return
	}
	if req == nil || req.TargetUid < 1 {
		p.ResponseCode(session, code.LoginFail)
		return
	}
	now := gtime.Now().UnixMilli()
	inviteID := cherrySnowflake.NextID()
	expireAt := now + inviteValidMS
	c := persistence.Invite(context.Background(), session.Uid, req.TargetUid, inviteID, expireAt, now)
	if c != code.OK {
		p.reject(session, "invite", c)
		return
	}
	roster, sc := persistence.PartyStateOf(context.Background(), session.Uid, now)
	partyID := int64(0)
	if sc == code.OK {
		partyID = roster.PartyID
	} else {
		clog.Warnf("party invite state uid=%d code=%d", session.Uid, sc)
	}
	publish(p, &protocol.PartyDeliver{
		Uids: []int64{req.TargetUid},
		Invite: &protocol.PartyInvite{
			InviteId:  inviteID,
			PartyId:   partyID,
			LeaderUid: session.Uid,
			ExpireAt:  expireAt,
		},
	})
	p.Response(session, &emptypb.Empty{})
}

func (p *actorParty) answer(session *cproto.Session, req *protocol.PartyAnswerRequest) {
	if !p.requireEntered(session) {
		return
	}
	if req == nil || req.InviteId < 1 {
		p.ResponseCode(session, code.LoginFail)
		return
	}
	now := gtime.Now().UnixMilli()
	st, c := persistence.Answer(context.Background(), session.Uid, req.InviteId, req.Accept, now)
	if c != code.OK {
		p.reject(session, "answer", c)
		return
	}
	view := protoState(st)
	publish(p, &protocol.PartyDeliver{Uids: memberUIDs(st), State: view})
	p.Response(session, view)
}

func (p *actorParty) leave(session *cproto.Session, _ *protocol.None) {
	if !p.requireEntered(session) {
		return
	}
	now := gtime.Now().UnixMilli()
	ctx := context.Background()
	// 只剩一人导致解散时，Leave 的 rest 是空名单，不含留下的那个人。先记下离开前的成员。
	before, _ := persistence.PartyStateOf(ctx, session.Uid, now)
	_, rest, c := persistence.Leave(ctx, session.Uid, now)
	if c != code.OK {
		p.reject(session, "leave", c)
		return
	}
	if rest.PartyID != 0 {
		pushEmpty(p, []int64{session.Uid})
		PushRoster(p, rest)
	} else {
		pushEmpty(p, withUID(before.Members, session.Uid))
	}
	p.Response(session, &emptypb.Empty{})
}

func (p *actorParty) kick(session *cproto.Session, req *protocol.PartyKickRequest) {
	if !p.requireEntered(session) {
		return
	}
	if req == nil || req.MemberUid < 1 {
		p.ResponseCode(session, code.LoginFail)
		return
	}
	now := gtime.Now().UnixMilli()
	ctx := context.Background()
	before, _ := persistence.PartyStateOf(ctx, session.Uid, now)
	_, rest, c := persistence.Kick(ctx, session.Uid, req.MemberUid, now)
	if c != code.OK {
		p.reject(session, "kick", c)
		return
	}
	if rest.PartyID != 0 {
		pushEmpty(p, []int64{req.MemberUid})
		PushRoster(p, rest)
	} else {
		pushEmpty(p, withUID(withUID(before.Members, req.MemberUid), session.Uid))
	}
	p.Response(session, &emptypb.Empty{})
}

func (p *actorParty) state(session *cproto.Session, _ *protocol.None) {
	if !p.requireEntered(session) {
		return
	}
	st, c := persistence.PartyStateOf(context.Background(), session.Uid, gtime.Now().UnixMilli())
	if c != code.OK {
		p.reject(session, "state", c)
		return
	}
	p.Response(session, protoState(st))
}

// publish 只把变化推给本机已 Bind 的 uid。不按在线节点分组，也不调用其他游戏节点。
func publish(sender cfacade.IActor, req *protocol.PartyDeliver) {
	Deliver(sender, req)
}

// PushRoster 把当前名单推给仍在队里且本机 Bind 过的 uid。
func PushRoster(sender cfacade.IActor, st persistence.PartyState) {
	if st.PartyID == 0 || len(st.Members) == 0 {
		return
	}
	publish(sender, &protocol.PartyDeliver{Uids: memberUIDs(st), State: protoState(st)})
}

func pushEmpty(sender cfacade.IActor, uids []int64) {
	if len(uids) == 0 {
		return
	}
	publish(sender, &protocol.PartyDeliver{Uids: uids, State: &protocol.PartyState{}})
}

func protoState(st persistence.PartyState) *protocol.PartyState {
	return &protocol.PartyState{
		PartyId:   st.PartyID,
		LeaderUid: st.LeaderUID,
		Members:   memberUIDs(st),
	}
}

func memberUIDs(st persistence.PartyState) []int64 {
	return append([]int64(nil), st.Members...)
}

func withUID(members []int64, uid int64) []int64 {
	out := append([]int64(nil), members...)
	if uid > 0 && !containsUID(out, uid) {
		out = append(out, uid)
	}
	return out
}

func containsUID(uids []int64, uid int64) bool {
	for _, id := range uids {
		if id == uid {
			return true
		}
	}
	return false
}
