package bag

import (
	"context"
	"errors"
	"strconv"
	"time"

	clog "github.com/cherry-game/cherry/logger"
	"github.com/cherry-game/cherry/net/parser/pomelo"
	cproto "github.com/cherry-game/cherry/net/proto"
	cprofile "github.com/cherry-game/cherry/profile"
	gcruntime "github.com/example/mmo-server/gameconfig/pkg/runtime"
	"github.com/example/mmo-server/internal/code"
	"github.com/example/mmo-server/internal/gameapp/equip"
	"github.com/example/mmo-server/internal/gameapp/world"
	"github.com/example/mmo-server/internal/persistence"
	"github.com/example/mmo-server/internal/protocol"
	"github.com/example/mmo-server/internal/sessionkey"
)

// offlineUnloadDefault 是 profile 未配置或配成非正数时的离线卸载秒数。
const offlineUnloadDefault = 300

// offlineUnloadAfter 读取 bag.offline_unload_seconds。未配置时为 300 秒。
func offlineUnloadAfter() time.Duration {
	seconds := cprofile.GetConfig("bag").GetInt("offline_unload_seconds", offlineUnloadDefault)
	if seconds < 1 {
		seconds = offlineUnloadDefault
	}
	return time.Duration(seconds) * time.Second
}

// actorBag 背包玩法 Actor：玩家进场（Session 含 PlayerID）后可访问。
// 内存态是背包唯一权威：玩家操作只改内存，脏包由定时/关键事件整包 diff 落库。
type actorBag struct {
	pomelo.ActorBase

	playerID     int64             // 当前服务的角色 id（懒加载时确定）
	bags         map[int32]*memBag // bagType -> 内存背包
	offlineTimer uint64            // 离线卸载倒计时定时器 id，0 表示无
}

// OnInit 注册路由 list / add / remove / move / split / use，以及远程 offline / invalidate。
func (p *actorBag) OnInit() {
	p.Local().Register("list", p.list)
	p.Local().Register("add", p.add)
	p.Local().Register("remove", p.remove)
	p.Local().Register("move", p.move)
	p.Local().Register("split", p.split)
	p.Local().Register("use", p.use)
	p.Remote().Register("offline", p.offline)
	p.Remote().Register("flush", p.flush)
	p.Remote().Register("invalidate", p.invalidate)
	p.Remote().Register("load", p.load)
	p.Remote().Register("grant", p.grant)
	p.Remote().Register("deduct", p.deduct)
	p.Remote().Register("rewardDeliver", p.rewardDeliver)
	p.Remote().Register("rewardClaim", p.rewardClaim)
	p.Remote().Register("equipApply", p.equipApply)
	p.Remote().Register("equipEnhance", p.equipEnhance)
}

// playerIDFromSession 从已进场的 session 取出角色 id。
func (p *actorBag) playerIDFromSession(session *cproto.Session) (int64, bool) {
	if session == nil || !session.Contains(sessionkey.PlayerID) {
		return 0, false
	}
	playerID, err := strconv.ParseInt(session.GetString(sessionkey.PlayerID), 10, 64)
	if err != nil || playerID < 1 {
		return 0, false
	}
	return playerID, true
}

// ensureBag 懒加载指定背包到内存。角色切换（playerID 变化）时整体清空重载。
func (p *actorBag) ensureBag(playerID int64, bagType int32) (*memBag, error) {
	if p.bags == nil {
		p.bags = make(map[int32]*memBag)
	}
	if playerID != p.playerID {
		p.bags = make(map[int32]*memBag)
		p.playerID = playerID
	}
	mb, ok := p.bags[bagType]
	if ok {
		return mb, nil
	}
	rsp, err := persistence.LoadBagFromDB(context.Background(), p.playerID, bagType)
	if err != nil {
		return nil, err
	}
	mb = newMemBag()
	for _, it := range rsp.Items {
		if it == nil {
			continue
		}
		mb.slots[it.Slot] = memSlot{itemID: it.ItemId, count: it.Count, level: it.EnhanceLevel}
	}
	p.bags[bagType] = mb
	return mb, nil
}

// cancelOffline 重新进场时取消离线卸载倒计时。
func (p *actorBag) cancelOffline() {
	if p.offlineTimer != 0 {
		p.Timer().Remove(p.offlineTimer)
		p.offlineTimer = 0
	}
}

// offline 断线/下线：立即 flush 脏包，再起离线卸载倒计时。切图不走这里。
func (p *actorBag) offline() {
	p.flushDirty()
	p.cancelOffline()
	p.offlineTimer = p.Timer().AddOnce(offlineUnloadAfter(), func() {
		p.offlineTimer = 0
		p.flushDirty()
		equip.Unload(p.playerID)
		p.bags = make(map[int32]*memBag)
		p.playerID = 0
	})
}

// invalidate 外部路径直写 DB 后失效本角色内存，下次操作懒加载重读。
func (p *actorBag) invalidate() {
	p.bags = make(map[int32]*memBag)
	p.playerID = 0
	p.cancelOffline()
}

// flush 关键事件落库：立即 flush 当前角色脏包，不触发卸载倒计时（切图等仍在线场景）。
func (p *actorBag) flush() {
	p.flushDirty()
}

// load 进场预热：把该角色拥有物品的全部背包装进内存。失败退化为懒加载（首次操作再读）。
// 无参版本供 player enter 后 fire-and-forget；带 BagLoadRequest 时以请求里的 playerID 为准。
func (p *actorBag) load(req *protocol.BagLoadRequest) {
	playerID := int64(0)
	if req != nil {
		playerID = req.PlayerId
	}
	if playerID < 1 {
		playerID = p.playerID
	}
	if playerID < 1 {
		return
	}
	// 角色未变且已加载过则跳过（重连复用内存）。
	if playerID == p.playerID && p.bags != nil && len(p.bags) > 0 {
		p.cancelOffline()
		return
	}
	p.cancelOffline()
	p.playerID = playerID
	if p.bags == nil {
		p.bags = make(map[int32]*memBag)
	}
	types, err := persistence.BagTypesOwned(context.Background(), playerID)
	if err != nil {
		clog.Warnf("bag load types player_id=%d err=%v", playerID, err)
		return
	}
	for _, bt := range types {
		if _, err := p.ensureBag(playerID, bt); err != nil {
			clog.Warnf("bag load bag player_id=%d bag_type=%d err=%v", playerID, bt, err)
		}
	}
}

// grant GM 发奖（在线改内存）。playerID 与 bagType 以请求为准。
func (p *actorBag) grant(req *protocol.GmGrantRequest) (*protocol.GmGrantResponse, int32) {
	if req == nil || req.PlayerId < 1 || req.ItemId < 1 {
		return nil, code.GmBadRequest
	}
	count := req.Count
	if count < 1 {
		count = 1
	}
	var bagType int32
	var err error
	if req.BagType > 0 {
		bagType = req.BagType
	} else {
		bagType, err = memResolveBagType(req.ItemId)
		if err != nil {
			return nil, bagErrorCode(err)
		}
	}
	mb, err := p.ensureBag(req.PlayerId, bagType)
	if err != nil {
		return nil, code.BagLoadFail
	}
	if err := mb.add(bagType, req.ItemId, count); err != nil {
		return nil, bagErrorCode(err)
	}
	p.cancelOffline()
	return p.grantResponse(req.PlayerId, bagType, mb), code.OK
}

// deduct GM 扣道具（在线改内存）。itemId 优先，否则按槽位。
func (p *actorBag) deduct(req *protocol.GmDeductRequest) (*protocol.GmGrantResponse, int32) {
	if req == nil || req.PlayerId < 1 {
		return nil, code.GmBadRequest
	}
	if req.ItemId < 1 && req.BagType < 1 {
		return nil, code.GmBadRequest
	}
	count := req.Count
	if count < 1 {
		count = 1
	}
	var bagType int32
	var err error
	if req.ItemId > 0 {
		if req.BagType > 0 {
			bagType = req.BagType
		} else {
			bagType, err = memResolveBagType(req.ItemId)
			if err != nil {
				return nil, bagErrorCode(err)
			}
		}
	} else {
		bagType = req.BagType
	}
	mb, err := p.ensureBag(req.PlayerId, bagType)
	if err != nil {
		return nil, code.BagLoadFail
	}
	if req.ItemId > 0 {
		err = mb.removeByItemID(bagType, req.ItemId, count)
	} else {
		err = mb.removeBySlot(bagType, req.Slot, count)
	}
	if err != nil {
		return nil, bagErrorCode(err)
	}
	p.cancelOffline()
	return p.grantResponse(req.PlayerId, bagType, mb), code.OK
}

// grantResponse 组装 GmGrantResponse 并尽力推 onBagChange。
func (p *actorBag) grantResponse(playerID int64, bagType int32, mb *memBag) *protocol.GmGrantResponse {
	bag := mb.toProto(bagType)
	pushed := p.pushBagChange(bag)
	return &protocol.GmGrantResponse{BagType: bagType, Bag: bag, Pushed: pushed}
}

// pushBagChange 尽力推 onBagChange；未进场（无网关路径）时不推。
func (p *actorBag) pushBagChange(bag *protocol.BagListResponse) bool {
	uid, err := strconv.ParseInt(p.ActorID(), 10, 64)
	if err != nil || uid < 1 {
		return false
	}
	path, ok := world.AgentPath(uid)
	if !ok {
		return false
	}
	pomelo.PushWithUID(p, path, uid, "onBagChange", bag)
	return true
}

// rewardDeliver 击杀奖励发放（在线改内存）。整份放不下则写待领取，放得进则进内存背包并推 onBagChange。
func (p *actorBag) rewardDeliver(req *protocol.RewardDeliverRequest) (*protocol.RewardDeliverResponse, int32) {
	if req == nil || req.PlayerId < 1 || req.ItemId < 1 || req.Count < 1 {
		return nil, code.BagItemInvalid
	}
	bagType, err := memResolveBagType(req.ItemId)
	if err != nil {
		return nil, bagErrorCode(err)
	}
	mb, err := p.ensureBag(req.PlayerId, bagType)
	if err != nil {
		return nil, code.BagLoadFail
	}
	rsp := &protocol.RewardDeliverResponse{BagType: bagType}
	if mb.itemRoom(bagType, req.ItemId) < req.Count {
		claimID, err := persistence.CreateKillClaim(context.Background(), req.PlayerId, req.ItemId, req.Count, req.MonsterId)
		if err != nil {
			clog.Warnf("reward deliver claim fail player_id=%d err=%v", req.PlayerId, err)
			return nil, code.BagLoadFail
		}
		rsp.Placed = false
		rsp.ClaimId = claimID
		return rsp, code.OK
	}
	if err := mb.add(bagType, req.ItemId, req.Count); err != nil {
		return nil, bagErrorCode(err)
	}
	p.cancelOffline()
	rsp.Placed = true
	rsp.Bag = mb.toProto(bagType)
	p.pushBagChange(rsp.Bag)
	return rsp, code.OK
}

// rewardClaim 领取待领取（在线改内存）。背包仍满返回 ErrBagFull；否则删除待领取并进内存背包。
func (p *actorBag) rewardClaim(req *protocol.RewardClaimRequest) (*protocol.RewardClaimResponse, int32) {
	if req == nil || req.PlayerId < 1 || req.ClaimId < 1 {
		return nil, code.BagItemInvalid
	}
	row, err := persistence.GetRewardClaim(context.Background(), req.PlayerId, req.ClaimId)
	if err != nil {
		if errors.Is(err, persistence.ErrRewardClaimMissing) {
			return nil, code.ItemNotFound
		}
		clog.Warnf("reward claim lookup player_id=%d claim=%d err=%v", req.PlayerId, req.ClaimId, err)
		return nil, code.BagLoadFail
	}
	bagType, err := memResolveBagType(row.ItemID)
	if err != nil {
		return nil, bagErrorCode(err)
	}
	mb, err := p.ensureBag(req.PlayerId, bagType)
	if err != nil {
		return nil, code.BagLoadFail
	}
	if mb.itemRoom(bagType, row.ItemID) < row.Count {
		return nil, code.BagFull
	}
	if err := mb.add(bagType, row.ItemID, row.Count); err != nil {
		return nil, bagErrorCode(err)
	}
	if err := persistence.DeleteRewardClaim(context.Background(), req.PlayerId, req.ClaimId); err != nil {
		if errors.Is(err, persistence.ErrRewardClaimMissing) {
			return nil, code.ItemNotFound
		}
		clog.Warnf("reward claim delete player_id=%d claim=%d err=%v", req.PlayerId, req.ClaimId, err)
		return nil, code.BagLoadFail
	}
	p.cancelOffline()
	bag := mb.toProto(bagType)
	p.pushBagChange(bag)
	return &protocol.RewardClaimResponse{BagType: bagType, Bag: bag}, code.OK
}

// bagErrorCode 把内存背包错误映射为业务码（与 actor_bag.go respondBagError 一致）。
func bagErrorCode(err error) int32 {
	switch {
	case errors.Is(err, persistence.ErrBagInvalid):
		return code.BagItemInvalid
	case errors.Is(err, persistence.ErrBagNotEnough):
		return code.BagItemNotEnough
	case errors.Is(err, persistence.ErrBagSlotInvalid):
		return code.BagSlotInvalid
	case errors.Is(err, persistence.ErrBagFull):
		return code.BagFull
	case errors.Is(err, persistence.ErrItemNotFound):
		return code.ItemNotFound
	case errors.Is(err, persistence.ErrBagTypeMismatch):
		return code.BagTypeMismatch
	default:
		return code.BagLoadFail
	}
}

// flushDirty 把当前角色的脏背包和脏穿着放进同一次事务。
func (p *actorBag) flushDirty() {
	if p.playerID < 1 {
		return
	}
	var diffs []persistence.BagDiff
	var dirtyBags []*memBag
	if p.bags != nil {
		for bagType, mb := range p.bags {
			if !mb.dirty() {
				continue
			}
			upserts, removes := mb.diff()
			up := make([]persistence.BagSlotChange, 0, len(upserts))
			for slot, s := range upserts {
				up = append(up, persistence.BagSlotChange{Slot: slot, ItemID: s.itemID, Count: s.count, Level: s.level})
			}
			diffs = append(diffs, persistence.BagDiff{BagType: bagType, Upserts: up, Removes: removes})
			dirtyBags = append(dirtyBags, mb)
		}
	}
	worn, equipDirty := equip.PeekDirty(p.playerID)
	if len(diffs) == 0 && !equipDirty {
		return
	}
	rows := make([]persistence.EquipRow, 0, len(worn))
	for _, row := range worn {
		rows = append(rows, persistence.EquipRow{SlotID: row.SlotID, ItemID: row.ItemID, Level: row.Level})
	}
	if err := persistence.FlushPlayer(context.Background(), p.playerID, diffs, rows, equipDirty); err != nil {
		clog.Warnf("bag flush fail player_id=%d err=%v", p.playerID, err)
		return
	}
	for _, mb := range dirtyBags {
		mb.clearTouched()
	}
	if equipDirty {
		equip.AckDirty(p.playerID)
	}
}

// list 返回指定背包内容；bag_type 必填。
func (p *actorBag) list(session *cproto.Session, req *protocol.BagListRequest) {
	playerID, ok := p.playerIDFromSession(session)
	if !ok {
		p.ResponseCode(session, code.PlayerNotEntered)
		return
	}
	if req == nil || req.BagType < 1 {
		p.ResponseCode(session, code.BagSlotInvalid)
		return
	}
	p.cancelOffline()
	mb, err := p.ensureBag(playerID, req.BagType)
	if err != nil {
		clog.Warnf("bag list fail player_id=%d bag_type=%d err=%v", playerID, req.BagType, err)
		p.ResponseCode(session, code.BagLoadFail)
		return
	}
	p.Response(session, mb.toProto(req.BagType))
}

// add 发放物品：默认按 item.bag_type 自动路由；req.BagType>0 时显式指定（GM），仍严格校验类别。
func (p *actorBag) add(session *cproto.Session, req *protocol.BagAddRequest) {
	playerID, ok := p.playerIDFromSession(session)
	if !ok {
		p.ResponseCode(session, code.PlayerNotEntered)
		return
	}
	if req == nil {
		p.ResponseCode(session, code.BagItemInvalid)
		return
	}
	count := req.Count
	if count < 1 {
		count = 1
	}
	p.cancelOffline()

	var err error
	var bagType int32
	if req.BagType > 0 {
		bagType = req.BagType
	} else {
		bagType, err = memResolveBagType(req.ItemId)
		if err != nil {
			p.respondBagError(session, playerID, err)
			return
		}
	}
	mb, err := p.ensureBag(playerID, bagType)
	if err != nil {
		p.respondBagError(session, playerID, err)
		return
	}
	if err := mb.add(bagType, req.ItemId, count); err != nil {
		p.respondBagError(session, playerID, err)
		return
	}
	p.respondBagMutate(session, playerID, bagType, mb)
}

// remove 扣除物品：bySlot=true 按槽位扣减，否则按 itemId 跨槽合计扣减。
func (p *actorBag) remove(session *cproto.Session, req *protocol.BagRemoveRequest) {
	playerID, ok := p.playerIDFromSession(session)
	if !ok {
		p.ResponseCode(session, code.PlayerNotEntered)
		return
	}
	if req == nil {
		p.ResponseCode(session, code.BagItemInvalid)
		return
	}
	if req.BagType < 1 {
		p.ResponseCode(session, code.BagSlotInvalid)
		return
	}
	count := req.Count
	if count < 1 {
		count = 1
	}
	p.cancelOffline()
	mb, err := p.ensureBag(playerID, req.BagType)
	if err != nil {
		p.respondBagError(session, playerID, err)
		return
	}
	if req.BySlot {
		err = mb.removeBySlot(req.BagType, req.Slot, count)
	} else {
		err = mb.removeByItemID(req.BagType, req.ItemId, count)
	}
	if err != nil {
		p.respondBagError(session, playerID, err)
		return
	}
	p.respondBagMutate(session, playerID, req.BagType, mb)
}

func (p *actorBag) move(session *cproto.Session, req *protocol.BagMoveRequest) {
	playerID, ok := p.playerIDFromSession(session)
	if !ok {
		p.ResponseCode(session, code.PlayerNotEntered)
		return
	}
	if req == nil {
		p.ResponseCode(session, code.BagItemInvalid)
		return
	}
	if req.BagType < 1 {
		p.ResponseCode(session, code.BagSlotInvalid)
		return
	}
	p.cancelOffline()
	mb, err := p.ensureBag(playerID, req.BagType)
	if err != nil {
		p.respondBagError(session, playerID, err)
		return
	}
	if err := mb.move(req.BagType, req.FromSlot, req.ToSlot); err != nil {
		p.respondBagError(session, playerID, err)
		return
	}
	p.respondBagMutate(session, playerID, req.BagType, mb)
}

// use 使用槽位上的 1 个道具。先挂 Buff，成功后才扣数量并推 onBagChange。
func (p *actorBag) use(session *cproto.Session, req *protocol.BagUseRequest) {
	playerID, ok := p.playerIDFromSession(session)
	if !ok {
		p.ResponseCode(session, code.PlayerNotEntered)
		return
	}
	if req == nil || req.BagType < 1 {
		p.ResponseCode(session, code.BagSlotInvalid)
		return
	}
	p.cancelOffline()
	mb, err := p.ensureBag(playerID, req.BagType)
	if err != nil {
		p.respondBagError(session, playerID, err)
		return
	}
	item, ok := mb.slots[req.Slot]
	if !ok || item.itemID < 1 {
		p.ResponseCode(session, code.BagSlotInvalid)
		return
	}
	def, ok := gcruntime.Get(item.itemID)
	if !ok {
		p.ResponseCode(session, code.ItemNotFound)
		return
	}
	if c := ApplyUse(session.Uid, def.UseBuffID); c != code.OK {
		p.ResponseCode(session, c)
		return
	}
	if err := mb.removeBySlot(req.BagType, req.Slot, 1); err != nil {
		p.respondBagError(session, playerID, err)
		return
	}
	p.respondBagMutate(session, playerID, req.BagType, mb)
}

func (p *actorBag) split(session *cproto.Session, req *protocol.BagSplitRequest) {
	playerID, ok := p.playerIDFromSession(session)
	if !ok {
		p.ResponseCode(session, code.PlayerNotEntered)
		return
	}
	if req == nil {
		p.ResponseCode(session, code.BagItemInvalid)
		return
	}
	if req.BagType < 1 {
		p.ResponseCode(session, code.BagSlotInvalid)
		return
	}
	count := req.Count
	if count < 1 {
		p.ResponseCode(session, code.BagItemInvalid)
		return
	}
	p.cancelOffline()
	mb, err := p.ensureBag(playerID, req.BagType)
	if err != nil {
		p.respondBagError(session, playerID, err)
		return
	}
	if err := mb.split(req.BagType, req.FromSlot, count); err != nil {
		p.respondBagError(session, playerID, err)
		return
	}
	p.respondBagMutate(session, playerID, req.BagType, mb)
}

func (p *actorBag) respondBagError(session *cproto.Session, playerID int64, err error) {
	switch {
	case errors.Is(err, persistence.ErrBagInvalid):
		p.ResponseCode(session, code.BagItemInvalid)
	case errors.Is(err, persistence.ErrBagNotEnough):
		p.ResponseCode(session, code.BagItemNotEnough)
	case errors.Is(err, persistence.ErrBagSlotInvalid):
		p.ResponseCode(session, code.BagSlotInvalid)
	case errors.Is(err, persistence.ErrBagFull):
		p.ResponseCode(session, code.BagFull)
	case errors.Is(err, persistence.ErrItemNotFound):
		p.ResponseCode(session, code.ItemNotFound)
	case errors.Is(err, persistence.ErrBagTypeMismatch):
		p.ResponseCode(session, code.BagTypeMismatch)
	default:
		clog.Warnf("bag op fail player_id=%d err=%v", playerID, err)
		p.ResponseCode(session, code.BagLoadFail)
	}
}

// respondBagMutate 变更成功后：RPC 返回内存态 BagListResponse，并 Push onBagChange（内容相同）。
func (p *actorBag) respondBagMutate(session *cproto.Session, playerID int64, bagType int32, mb *memBag) {
	bag := mb.toProto(bagType)
	p.Response(session, bag)
	p.Push(session, "onBagChange", bag)
}
