package equip

import (
	"strconv"

	cfacade "github.com/cherry-game/cherry/facade"
	clog "github.com/cherry-game/cherry/logger"
	"github.com/cherry-game/cherry/net/parser/pomelo"
	cproto "github.com/cherry-game/cherry/net/proto"
	"github.com/example/mmo-server/internal/code"
	"github.com/example/mmo-server/internal/gameapp/world"
	"github.com/example/mmo-server/internal/protocol"
	"github.com/example/mmo-server/internal/sessionkey"
)

// ActorEquips 装备根 Actor。实际状态由背包 Actor 持有，这里只做协议适配和转发。
type ActorEquips struct {
	pomelo.ActorBase
}

// AliasID 路由前缀 game.equip。
func (p *ActorEquips) AliasID() string { return "equip" }

// OnFindChild 按 uid 创建穿脱 Actor。框架按需建子 Actor，没有这个钩子消息会被丢弃。
func (p *ActorEquips) OnFindChild(msg *cfacade.Message) (cfacade.IActor, bool) {
	child, err := p.Child().Create(msg.TargetPath().ChildID, &actorEquip{})
	if err != nil {
		clog.Warnf("create equip child fail: %v", err)
		return nil, false
	}
	return child, true
}

// actorEquip 处理一名玩家的穿脱。未进场返回 40009。
// 穿着内存连同背包格子都在背包 Actor 里，这里只把请求翻译成 EquipApply 后转发，
// 保证穿脱和落库在同一个邮箱里串行，业务层不需要锁。
type actorEquip struct {
	pomelo.ActorBase
}

// OnInit 注册 wear、unwear、equips、enhance。
func (p *actorEquip) OnInit() {
	p.Local().Register("wear", p.wear)
	p.Local().Register("unwear", p.unwear)
	p.Local().Register("equips", p.equips)
	p.Local().Register("enhance", p.enhance)
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
	p.finish(session, apply)
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
	p.finish(session, apply)
}

func (p *actorEquip) enhance(session *cproto.Session, req *protocol.EquipEnhanceRequest) {
	playerID, ok := playerID(session)
	if !ok {
		p.ResponseCode(session, code.PlayerNotEntered)
		return
	}
	if req == nil || req.SlotId < 1 {
		p.ResponseCode(session, code.EquipSlotInvalid)
		return
	}
	apply := &protocol.EquipEnhanceApply{
		PlayerId: playerID, Uid: session.Uid, SlotId: req.SlotId, UseProtect: req.UseProtect,
	}
	var result protocol.EquipEnhanceResult
	rc := p.CallWait(cfacade.NewChildPath(p.App().NodeID(), "bag", session.Uid), "equipEnhance", apply, &result)
	if rc != code.OK {
		p.ResponseCode(session, rc)
		return
	}
	p.Response(session, &protocol.EquipEnhanceResponse{
		Success: result.Success, Dropped: result.Dropped, ProtectUsed: result.ProtectUsed,
		Level: result.Level, Attrs: result.Attrs,
	})
	push(p, session.Uid, result.State, result.Bag)
}

func (p *actorEquip) equips(session *cproto.Session, _ *protocol.None) {
	playerID, ok := playerID(session)
	if !ok {
		p.ResponseCode(session, code.PlayerNotEntered)
		return
	}
	var state protocol.EquipState
	rc := p.CallWait(cfacade.NewChildPath(p.App().NodeID(), "bag", session.Uid), "equipState",
		&protocol.EquipStateRequest{PlayerId: playerID, Uid: session.Uid}, &state)
	if rc != code.OK {
		p.ResponseCode(session, rc)
		return
	}
	p.Response(session, &state)
}

// finish 把一次穿脱转给背包 Actor，拿回穿着和背包后回包并推送。
func (p *actorEquip) finish(session *cproto.Session, apply *protocol.EquipApply) {
	var result protocol.EquipApplyResult
	rc := p.CallWait(cfacade.NewChildPath(p.App().NodeID(), "bag", session.Uid), "equipApply", apply, &result)
	if rc != code.OK {
		p.ResponseCode(session, rc)
		return
	}
	p.Response(session, result.State)
	push(p, session.Uid, result.State, result.Bag)
}

func push(sender cfacade.IActor, uid int64, state *protocol.EquipState, bag *protocol.BagListResponse) {
	if state == nil {
		return
	}
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
