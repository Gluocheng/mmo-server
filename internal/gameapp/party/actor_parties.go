package party

import (
	"context"

	cfacade "github.com/cherry-game/cherry/facade"
	clog "github.com/cherry-game/cherry/logger"
	"github.com/cherry-game/cherry/net/parser/pomelo"
	"github.com/example/mmo-server/internal/persistence"
	"github.com/example/mmo-server/internal/protocol"
	"google.golang.org/protobuf/types/known/emptypb"
)

// 客户端推送路由。InviteId 为 0 的邀请同样走 onPartyInvite，用来清掉邀请。
const (
	routeOnParty       = "onParty"
	routeOnPartyInvite = "onPartyInvite"
)

// pushWithUID 把消息推到网关。单测替换它，避免拉起集群。
var pushWithUID = pomelo.PushWithUID

// callRemote 调其他游戏节点的 party.deliver。单测替换它，避免拉起 NATS。
var callRemote func(targetPath, funcName string, arg, reply any) int32

// onlineNode 读取 party:online。单测替换它，避免和持久化测试抢同一个 Redis。
var onlineNode = persistence.OnlineNode

// ActorParties 组队根 Actor。按 uid 创建子 Actor，deliver 只推本机进场表里的人。
type ActorParties struct {
	pomelo.ActorBase
}

// AliasID 路由前缀 game.party。
func (p *ActorParties) AliasID() string {
	return "party"
}

// OnInit 注册远程方法 deliver。
func (p *ActorParties) OnInit() {
	p.Remote().Register("deliver", p.deliver)
}

// OnFindChild 按 uid 创建组队子 Actor。
func (p *ActorParties) OnFindChild(msg *cfacade.Message) (cfacade.IActor, bool) {
	childID := msg.TargetPath().ChildID
	childActor, err := p.Child().Create(childID, &actorParty{})
	if err != nil {
		clog.Warnf("create party child fail: %v", err)
		return nil, false
	}
	return childActor, true
}

// deliver 按本机进场表推送。不在 Path 里的 uid 跳过。
func (p *ActorParties) deliver(req *protocol.PartyDeliver) {
	Deliver(p, req)
}

// Deliver 把 PartyDeliver 推给本机已 Bind 的 uid。
// State 非空时推 onParty；Invite 非空时推 onPartyInvite，InviteId 为 0 也推。
// 两边都有则各推一次。uid 未 Bind 则跳过。推送失败不回滚 Redis。
func Deliver(sender cfacade.IActor, req *protocol.PartyDeliver) {
	if req == nil {
		return
	}
	for _, uid := range req.Uids {
		path, ok := Path(uid)
		if !ok {
			continue
		}
		if req.State != nil {
			pushWithUID(sender, path, uid, routeOnParty, req.State)
		}
		if req.Invite != nil {
			pushWithUID(sender, path, uid, routeOnPartyInvite, req.Invite)
		}
	}
}

// Dispatch 按 party:online 把同一份推送分到各游戏节点。
// 本节点直接 Deliver。其他节点 CallWait `{nodeID}.party` 的 deliver。
// 没有在线键的 uid（宽限中）不投递。推送失败不回滚 Redis。
func Dispatch(sender cfacade.IActor, localNode string, req *protocol.PartyDeliver) {
	if req == nil || len(req.Uids) == 0 {
		return
	}
	groups := map[string][]int64{}
	var order []string
	for _, uid := range req.Uids {
		node, err := onlineNode(context.Background(), uid)
		if err != nil || node == "" {
			continue
		}
		if _, ok := groups[node]; !ok {
			order = append(order, node)
		}
		groups[node] = append(groups[node], uid)
	}
	for _, node := range order {
		part := &protocol.PartyDeliver{Uids: groups[node], State: req.State, Invite: req.Invite}
		if node == localNode {
			Deliver(sender, part)
			continue
		}
		target := node + ".party"
		reply := &emptypb.Empty{}
		var rc int32
		if callRemote != nil {
			rc = callRemote(target, "deliver", part, reply)
		} else if sender != nil {
			rc = sender.CallWait(target, "deliver", part, reply)
		}
		if rc != 0 {
			clog.Warnf("party deliver node=%s code=%d", node, rc)
		}
	}
}
