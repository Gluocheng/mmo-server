package equip

import (
	cfacade "github.com/cherry-game/cherry/facade"
	clog "github.com/cherry-game/cherry/logger"
	"github.com/cherry-game/cherry/net/parser/pomelo"
	cproto "github.com/cherry-game/cherry/net/proto"
	"github.com/example/mmo-server/internal/code"
	"github.com/example/mmo-server/internal/gameapp/world"
	"github.com/example/mmo-server/internal/protocol"
	"github.com/example/mmo-server/internal/sessionkey"
	"strconv"
)

// ActorEquips 装备根 Actor。按 uid 创建子 Actor。
type ActorEquips struct {
	pomelo.ActorBase
}

// AliasID 路由前缀 game.equip。
func (p *ActorEquips) AliasID() string { return "equip" }

// OnFindChild 按 uid 创建穿脱 Actor。
func (p *ActorEquips) OnFindChild(msg *cfacade.Message) (cfacade.IActor, bool) {
	childID := msg.TargetPath().ChildID
	child, err := p.Child().Create(childID, &actorEquip{})
	if err != nil {
		clog.Warnf("create equip child fail: %v", err)
		return nil, false
	}
	return child, true
}

// actorEquip 处理一名玩家的穿脱。未进场返回 40009。
type actorEquip struct {
	pomelo.ActorBase
}

// OnInit 注册 wear、unwear、equips。
func (p *actorEquip) OnInit() {
	p.Local().Register("wear", p.wear)
	p.Local().Register("unwear", p.unwear)
	p.Local().Register("equips", p.equips)
}

func (p *actorEquip) wear(session *cproto.Session, req *protocol.EquipWearRequest) {
	playerID, ok := playerID(session)
	if !ok {
		p.ResponseCode(session, code.PlayerNotEntered)
		return
	}
	if req == nil {
		p.ResponseCode(session, code.EquipInvalid)
		return
	}
	bagType := req.BagType
	if bagType < 1 {
		bagType = BagType
	}
	apply := &protocol.EquipApply{
		PlayerId: playerID, Uid: session.Uid, Op: opWear,
		BagType: bagType, BagSlot: req.Slot, TargetSlot: req.TargetSlot,
	}
	p.finish(session, playerID, apply)
}

func (p *actorEquip) unwear(session *cproto.Session, req *protocol.EquipUnwearRequest) {
	playerID, ok := playerID(session)
	if !ok {
		p.ResponseCode(session, code.PlayerNotEntered)
		return
	}
	if req == nil || req.SlotId < 1 {
		p.ResponseCode(session, code.EquipSlotInvalid)
		return
	}
	apply := &protocol.EquipApply{
		PlayerId: playerID, Uid: session.Uid, Op: opUnwear,
		BagType: BagType, TargetSlot: req.SlotId,
	}
	p.finish(session, playerID, apply)
}

func (p *actorEquip) equips(session *cproto.Session, _ *protocol.None) {
	playerID, ok := playerID(session)
	if !ok {
		p.ResponseCode(session, code.PlayerNotEntered)
		return
	}
	state, c := State(playerID)
	if c != code.OK {
		p.ResponseCode(session, c)
		return
	}
	p.Response(session, state)
}

func (p *actorEquip) finish(session *cproto.Session, playerID int64, apply *protocol.EquipApply) {
	var bag protocol.BagListResponse
	rc := p.CallWait(cfacade.NewChildPath(p.App().NodeID(), "bag", session.Uid), "equipApply", apply, &bag)
	if rc != code.OK {
		p.ResponseCode(session, rc)
		return
	}
	state, c := State(playerID)
	if c != code.OK {
		p.ResponseCode(session, c)
		return
	}
	p.Response(session, state)
	push(p, session.Uid, state, &bag)
}

func push(sender cfacade.IActor, uid int64, state *protocol.EquipState, bag *protocol.BagListResponse) {
	path, ok := world.AgentPath(uid)
	if !ok || path == "" || sender == nil {
		return
	}
	pomelo.PushWithUID(sender, path, uid, "onEquip", state)
	if bag != nil {
		pomelo.PushWithUID(sender, path, uid, "onBagChange", bag)
	}
}

func playerID(session *cproto.Session) (int64, bool) {
	if session == nil || !session.Contains(sessionkey.PlayerID) {
		return 0, false
	}
	id, err := strconv.ParseInt(session.GetString(sessionkey.PlayerID), 10, 64)
	if err != nil || id < 1 {
		return 0, false
	}
	return id, true
}
