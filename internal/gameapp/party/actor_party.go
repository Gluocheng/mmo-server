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
	replaced, partyID, c := persistence.Invite(context.Background(), session.Uid, req.TargetUid, inviteID, expireAt, now)
	if c != code.OK {
		p.reject(session, "invite", c)
		return
	}
	if replaced {
		publish(p, &protocol.PartyDeliver{
			Uids:   []int64{req.TargetUid},
			Invite: &protocol.PartyInvite{InviteId: 0},
		})
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
	st, c, cleared := persistence.Answer(context.Background(), session.Uid, req.InviteId, req.Accept, now)
	if cleared {
		publish(p, &protocol.PartyDeliver{
			Uids:   []int64{session.Uid},
			Invite: &protocol.PartyInvite{InviteId: 0},
		})
	}
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
	_, rest, notify, c := persistence.Leave(context.Background(), session.Uid, now)
	pushChange(p, rest, notify)
	if c != code.OK {
		p.reject(session, "leave", c)
		return
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
	_, rest, notify, c := persistence.Kick(context.Background(), session.Uid, req.MemberUid, now)
	pushChange(p, rest, notify)
	if c != code.OK {
		p.reject(session, "kick", c)
		return
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

// publish 按在线节点投递。本节点走 Deliver，其他节点 CallWait。
func publish(sender cfacade.IActor, req *protocol.PartyDeliver) {
	node := ""
	if sender != nil {
		node = sender.Path().NodeID
	}
	Dispatch(sender, node, req)
}

// NotifyEnter 在进场写完在线节点后推送。
// 宽限到期离队时，被摘掉的人收到空名单，留下的人收到新名单。
// 回到原位时只推当前名单。BeginGrace 不走这里。
func NotifyEnter(sender cfacade.IActor, entered persistence.PartyEnter) {
	if len(entered.Dropped) > 0 {
		pushEmpty(sender, entered.Dropped)
	}
	if entered.Rest.PartyID != 0 {
		PushRoster(sender, entered.Rest)
	}
}

// PushRoster 把当前名单按在线节点投递。没有在线键的成员会被跳过。
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

// pushChange 给留下的人推新名单，给离开的人推空名单。解散时 notify 里的人全部收到空名单。
func pushChange(sender cfacade.IActor, rest persistence.PartyState, notify []int64) {
	if len(notify) == 0 {
		return
	}
	if rest.PartyID != 0 {
		PushRoster(sender, rest)
		pushEmpty(sender, absent(notify, rest.Members))
		return
	}
	pushEmpty(sender, notify)
}

func absent(all, keep []int64) []int64 {
	out := make([]int64, 0)
	for _, uid := range all {
		if !containsUID(keep, uid) {
			out = append(out, uid)
		}
	}
	return out
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

func containsUID(uids []int64, uid int64) bool {
	for _, id := range uids {
		if id == uid {
			return true
		}
	}
	return false
}
