package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/example/mmo-server/internal/code"
	"github.com/redis/go-redis/v9"
)

// 队伍人数上限。接受第 5 人时返回队伍已满；宽限未过期的成员仍占名额。
const partyMaxMembers = 4

// 断线宽限。BeginGrace 写成 now+partyGraceMS；只有 now 大于 graceUntil 才按离队摘掉。
// graceUntil 为 0 表示没有宽限（含进程崩溃没来得及写入），不会自动过期。
const partyGraceMS int64 = 60_000

// 邀请键的 Redis TTL。逻辑是否过期以 expireAt 与 now 比较，不能只靠这条 TTL。
const partyInviteTTL = 30 * time.Second

// errPartyRetry 表示监视键在事务外被改过，或预先收集的键已经不够，需要重试。
var errPartyRetry = errors.New("队伍名单已变化")

// PartyState 是一支队伍的对外名单。
// Members 按加入顺序，包含宽限未过期的成员，不含已过 graceUntil 的成员。
// 不在任何队伍中时 PartyID 为 0 且 Members 为空。
type PartyState struct {
	PartyID   int64
	LeaderUID int64
	Members   []int64
}

// partySeat 是队伍键里的一个席位。GraceUntil 为 0 表示没有宽限。
type partySeat struct {
	UID        int64 `json:"uid"`
	GraceUntil int64 `json:"graceUntil"`
}

// partyRecord 整份存在一个字符串里，事务中一次 SET 写完队长和成员。
type partyRecord struct {
	Leader  int64       `json:"leader"`
	Members []partySeat `json:"members"`
}

type partyInviteRecord struct {
	InviteID int64 `json:"inviteId"`
	PartyID  int64 `json:"partyId"`
	Leader   int64 `json:"leader"`
	ExpireAt int64 `json:"expireAt"`
}

type redisGetter interface {
	Get(ctx context.Context, key string) *redis.StringCmd
}

func partyKey(id int64) string {
	return fmt.Sprintf("%s:party:%d", KeyPrefix(), id)
}

func partyUIDKey(uid int64) string {
	return fmt.Sprintf("%s:party:uid:%d", KeyPrefix(), uid)
}

func partyInviteKey(uid int64) string {
	return fmt.Sprintf("%s:party:invite:%d", KeyPrefix(), uid)
}

func partyOnlineKey(uid int64) string {
	return fmt.Sprintf("%s:party:online:%d", KeyPrefix(), uid)
}

func ctxOrBG(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func readPartyRaw(ctx context.Context, cmd redisGetter, partyID int64) (string, error) {
	raw, err := cmd.Get(ctx, partyKey(partyID)).Result()
	if errors.Is(err, redis.Nil) {
		return "", nil
	}
	return raw, err
}

func readUIDParty(ctx context.Context, cmd redisGetter, uid int64) (int64, error) {
	raw, err := cmd.Get(ctx, partyUIDKey(uid)).Result()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(raw, 10, 64)
}

func readOnline(ctx context.Context, cmd redisGetter, uid int64) (string, error) {
	raw, err := cmd.Get(ctx, partyOnlineKey(uid)).Result()
	if errors.Is(err, redis.Nil) {
		return "", nil
	}
	return raw, err
}

func readInviteRaw(ctx context.Context, cmd redisGetter, uid int64) (string, error) {
	raw, err := cmd.Get(ctx, partyInviteKey(uid)).Result()
	if errors.Is(err, redis.Nil) {
		return "", nil
	}
	return raw, err
}

func parseParty(raw string) (*partyRecord, error) {
	if raw == "" {
		return nil, nil
	}
	var rec partyRecord
	if err := json.Unmarshal([]byte(raw), &rec); err != nil {
		return nil, err
	}
	return &rec, nil
}

func parseInvite(raw string) (*partyInviteRecord, error) {
	if raw == "" {
		return nil, nil
	}
	var inv partyInviteRecord
	if err := json.Unmarshal([]byte(raw), &inv); err != nil {
		return nil, err
	}
	return &inv, nil
}

func (rec *partyRecord) has(uid int64) bool {
	if rec == nil {
		return false
	}
	for _, m := range rec.Members {
		if m.UID == uid {
			return true
		}
	}
	return false
}

func (rec *partyRecord) view(partyID int64) PartyState {
	if rec == nil || partyID == 0 || len(rec.Members) == 0 {
		return PartyState{}
	}
	members := make([]int64, len(rec.Members))
	for i, m := range rec.Members {
		members[i] = m.UID
	}
	return PartyState{PartyID: partyID, LeaderUID: rec.Leader, Members: members}
}

// applyPrune 按离队规则摘掉已过宽限的成员。
// 剩下不少于 2 人且队长已离开时，队长交给剩余成员里最早加入的一位。
// 剩下不足 2 人则整队解散。graceUntil 为 0 或 now 尚未超过它时保留原下标。
func applyPrune(rec *partyRecord, now int64) (next *partyRecord, drops []int64, dissolved, changed bool) {
	if rec == nil {
		return nil, nil, false, false
	}
	keep := make([]partySeat, 0, len(rec.Members))
	for _, m := range rec.Members {
		if m.GraceUntil > 0 && now > m.GraceUntil {
			drops = append(drops, m.UID)
			continue
		}
		keep = append(keep, m)
	}
	if len(drops) == 0 {
		return rec, nil, false, false
	}
	if len(keep) < 2 {
		return nil, drops, true, true
	}
	leader := rec.Leader
	if !seatsHave(keep, leader) {
		leader = keep[0].UID
	}
	return &partyRecord{Leader: leader, Members: keep}, drops, false, true
}

func seatsHave(seats []partySeat, uid int64) bool {
	for _, m := range seats {
		if m.UID == uid {
			return true
		}
	}
	return false
}

func withoutSeat(seats []partySeat, uid int64) []partySeat {
	kept := make([]partySeat, 0, len(seats))
	for _, m := range seats {
		if m.UID != uid {
			kept = append(kept, m)
		}
	}
	return kept
}

// partyWrite 聚合同一次 Redis 事务要提交的键。
type partyWrite struct {
	set       map[int64]*partyRecord
	del       []int64
	bind      map[int64]int64
	unbind    []int64
	onlineSet map[int64]string
	onlineDel []int64
	inviteSet map[int64]partyInviteRecord
	inviteDel []int64
}

func newPartyWrite() *partyWrite {
	return &partyWrite{
		set:       map[int64]*partyRecord{},
		bind:      map[int64]int64{},
		onlineSet: map[int64]string{},
		inviteSet: map[int64]partyInviteRecord{},
	}
}

func (w *partyWrite) empty() bool {
	return len(w.set) == 0 && len(w.del) == 0 && len(w.bind) == 0 && len(w.unbind) == 0 &&
		len(w.onlineSet) == 0 && len(w.onlineDel) == 0 && len(w.inviteSet) == 0 && len(w.inviteDel) == 0
}

// putPrune 把过期摘人的结果放进本次事务。解散时清掉全体成员的 uid 索引。
func (w *partyWrite) putPrune(partyID int64, prev, next *partyRecord, dissolved bool) {
	if dissolved || next == nil {
		w.dissolve(partyID, prev.Members)
		return
	}
	w.set[partyID] = next
	for _, m := range prev.Members {
		if !next.has(m.UID) {
			w.unbind = append(w.unbind, m.UID)
		}
	}
}

// dissolved 表示这次事务已经删掉该队伍，可以复用同一个 partyID。
// 名单还在（包括刚摘掉过期成员后的剩余名单）时不能覆盖。
func (w *partyWrite) dissolved(partyID int64) bool {
	if _, ok := w.set[partyID]; ok {
		return false
	}
	for _, id := range w.del {
		if id == partyID {
			return true
		}
	}
	return false
}

// dissolve 删除队伍键，并解除当时仍在名单上的 uid 索引。
func (w *partyWrite) dissolve(partyID int64, members []partySeat) {
	delete(w.set, partyID)
	w.del = append(w.del, partyID)
	for _, m := range members {
		delete(w.bind, m.UID)
		w.unbind = append(w.unbind, m.UID)
	}
}

// removeMember 按主动离队/踢人规则去掉一人。不足 2 人时解散，返回 nil。
func (w *partyWrite) removeMember(partyID int64, base *partyRecord, target int64) *partyRecord {
	kept := withoutSeat(base.Members, target)
	if len(kept) < 2 {
		w.dissolve(partyID, base.Members)
		return nil
	}
	leader := base.Leader
	if target == leader || !seatsHave(kept, leader) {
		leader = kept[0].UID
	}
	updated := &partyRecord{Leader: leader, Members: kept}
	w.set[partyID] = updated
	delete(w.bind, target)
	w.unbind = append(w.unbind, target)
	return updated
}

func (w *partyWrite) flush(ctx context.Context, pipe redis.Pipeliner) error {
	gone := map[int64]struct{}{}
	for _, id := range w.del {
		if _, ok := w.set[id]; ok {
			continue
		}
		if _, ok := gone[id]; ok {
			continue
		}
		gone[id] = struct{}{}
		pipe.Del(ctx, partyKey(id))
	}
	for id, rec := range w.set {
		raw, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		pipe.Set(ctx, partyKey(id), raw, 0)
	}
	unbound := map[int64]struct{}{}
	for _, uid := range w.unbind {
		if _, ok := w.bind[uid]; ok {
			continue
		}
		if _, ok := unbound[uid]; ok {
			continue
		}
		unbound[uid] = struct{}{}
		pipe.Del(ctx, partyUIDKey(uid))
	}
	for uid, pid := range w.bind {
		pipe.Set(ctx, partyUIDKey(uid), strconv.FormatInt(pid, 10), 0)
	}
	for uid, node := range w.onlineSet {
		pipe.Set(ctx, partyOnlineKey(uid), node, 0)
	}
	off := map[int64]struct{}{}
	for _, uid := range w.onlineDel {
		if _, ok := w.onlineSet[uid]; ok {
			continue
		}
		if _, ok := off[uid]; ok {
			continue
		}
		off[uid] = struct{}{}
		pipe.Del(ctx, partyOnlineKey(uid))
	}
	for uid, inv := range w.inviteSet {
		raw, err := json.Marshal(inv)
		if err != nil {
			return err
		}
		pipe.Set(ctx, partyInviteKey(uid), raw, partyInviteTTL)
	}
	invDel := map[int64]struct{}{}
	for _, uid := range w.inviteDel {
		if _, ok := w.inviteSet[uid]; ok {
			continue
		}
		if _, ok := invDel[uid]; ok {
			continue
		}
		invDel[uid] = struct{}{}
		pipe.Del(ctx, partyInviteKey(uid))
	}
	return nil
}

func execPartyWrite(ctx context.Context, tx *redis.Tx, w *partyWrite) error {
	if w == nil || w.empty() {
		return nil
	}
	_, err := tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		return w.flush(ctx, pipe)
	})
	return err
}

// partySnap 是事务开始前读到的键，用来决定 WATCH 集合并在回调里核对。
type partySnap struct {
	uidParty map[int64]int64
	partyRaw map[int64]string
	online   map[int64]string
	invite   map[int64]string
}

func newPartySnap() *partySnap {
	return &partySnap{
		uidParty: map[int64]int64{},
		partyRaw: map[int64]string{},
		online:   map[int64]string{},
		invite:   map[int64]string{},
	}
}

func (s *partySnap) trackUID(ctx context.Context, uid int64) error {
	if uid < 1 {
		return nil
	}
	pid, err := readUIDParty(ctx, rdb, uid)
	if err != nil {
		return err
	}
	s.uidParty[uid] = pid
	return nil
}

func (s *partySnap) trackParty(ctx context.Context, partyID int64) error {
	if partyID < 1 {
		return nil
	}
	raw, err := readPartyRaw(ctx, rdb, partyID)
	if err != nil {
		return err
	}
	s.partyRaw[partyID] = raw
	return nil
}

func (s *partySnap) trackMembers(ctx context.Context, partyID int64) error {
	if err := s.trackParty(ctx, partyID); err != nil {
		return err
	}
	rec, err := parseParty(s.partyRaw[partyID])
	if err != nil || rec == nil {
		return err
	}
	for _, m := range rec.Members {
		if err := s.trackUID(ctx, m.UID); err != nil {
			return err
		}
	}
	return nil
}

func (s *partySnap) trackUser(ctx context.Context, uid int64) error {
	if err := s.trackUID(ctx, uid); err != nil {
		return err
	}
	return s.trackMembers(ctx, s.uidParty[uid])
}

func (s *partySnap) trackOnline(ctx context.Context, uid int64) error {
	node, err := readOnline(ctx, rdb, uid)
	if err != nil {
		return err
	}
	s.online[uid] = node
	return nil
}

func (s *partySnap) trackInvite(ctx context.Context, uid int64) error {
	raw, err := readInviteRaw(ctx, rdb, uid)
	if err != nil {
		return err
	}
	s.invite[uid] = raw
	return nil
}

func (s *partySnap) keys() []string {
	keys := make([]string, 0, len(s.uidParty)+len(s.partyRaw)+len(s.online)+len(s.invite))
	seen := map[string]struct{}{}
	add := func(key string) {
		if key == "" {
			return
		}
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	for uid := range s.uidParty {
		add(partyUIDKey(uid))
	}
	for id := range s.partyRaw {
		add(partyKey(id))
	}
	for uid := range s.online {
		add(partyOnlineKey(uid))
	}
	for uid := range s.invite {
		add(partyInviteKey(uid))
	}
	return keys
}

func (s *partySnap) consistent(ctx context.Context, tx redisGetter) error {
	for uid, pid := range s.uidParty {
		cur, err := readUIDParty(ctx, tx, uid)
		if err != nil {
			return err
		}
		if cur != pid {
			return errPartyRetry
		}
	}
	for id, raw := range s.partyRaw {
		cur, err := readPartyRaw(ctx, tx, id)
		if err != nil {
			return err
		}
		if cur != raw {
			return errPartyRetry
		}
	}
	for uid, node := range s.online {
		cur, err := readOnline(ctx, tx, uid)
		if err != nil {
			return err
		}
		if cur != node {
			return errPartyRetry
		}
	}
	for uid, raw := range s.invite {
		cur, err := readInviteRaw(ctx, tx, uid)
		if err != nil {
			return err
		}
		if cur != raw {
			return errPartyRetry
		}
	}
	return nil
}

// commitWatched 在 WATCH 保护下改名单。队伍键与 uid 索引放进同一次 MULTI。
func commitWatched(ctx context.Context, load func() (*partySnap, error), mutate func(tx *redis.Tx) error) error {
	if rdb == nil {
		return fmt.Errorf("redis 不可用")
	}
	var err error
	for i := 0; i < 8; i++ {
		var snap *partySnap
		snap, err = load()
		if err != nil {
			return err
		}
		keys := snap.keys()
		if len(keys) == 0 {
			return fmt.Errorf("队伍监视键为空")
		}
		err = rdb.Watch(ctx, func(tx *redis.Tx) error {
			if err := snap.consistent(ctx, tx); err != nil {
				return err
			}
			return mutate(tx)
		}, keys...)
		if errors.Is(err, redis.TxFailedErr) || errors.Is(err, errPartyRetry) {
			continue
		}
		return err
	}
	if err == nil {
		return fmt.Errorf("队伍名单冲突")
	}
	return err
}

func loadUserParty(ctx context.Context, cmd redisGetter, uid int64) (int64, *partyRecord, error) {
	pid, err := readUIDParty(ctx, cmd, uid)
	if err != nil || pid == 0 {
		return pid, nil, err
	}
	raw, err := readPartyRaw(ctx, cmd, pid)
	if err != nil {
		return 0, nil, err
	}
	rec, err := parseParty(raw)
	if err != nil {
		return 0, nil, err
	}
	return pid, rec, nil
}

// activeAfterPrune 先摘过期成员。changed 时把结果写入 w，返回调用方还能看到的名单。
func activeAfterPrune(w *partyWrite, partyID int64, rec *partyRecord, now int64) (*partyRecord, bool) {
	next, _, dissolved, changed := applyPrune(rec, now)
	if !changed {
		return rec, false
	}
	w.putPrune(partyID, rec, next, dissolved)
	if dissolved {
		return nil, true
	}
	return next, true
}

// CreateParty 创建队伍：uid 成为队长和第一名成员，并写入在线节点。
// 已在队（含宽限未过期）返回 40060。宽限已过则先按离队摘掉，再允许创建。
// 存储层不返回未进场码 40009。
func CreateParty(ctx context.Context, uid, partyID int64, nodeID string, now int64) (PartyState, int32) {
	ctx = ctxOrBG(ctx)
	if rdb == nil || uid < 1 || partyID < 1 {
		return PartyState{}, code.LoginFail
	}
	var out PartyState
	var outCode int32
	err := commitWatched(ctx, func() (*partySnap, error) {
		snap := newPartySnap()
		if err := snap.trackUser(ctx, uid); err != nil {
			return nil, err
		}
		if err := snap.trackParty(ctx, partyID); err != nil {
			return nil, err
		}
		return snap, nil
	}, func(tx *redis.Tx) error {
		out = PartyState{}
		outCode = code.OK
		w := newPartyWrite()
		pid, rec, err := loadUserParty(ctx, tx, uid)
		if err != nil {
			return err
		}
		if pid > 0 && rec == nil {
			w.unbind = append(w.unbind, uid)
		}
		if rec != nil {
			active, _ := activeAfterPrune(w, pid, rec, now)
			if active != nil && active.has(uid) {
				out = active.view(pid)
				outCode = code.PartyAlreadyIn
				return execPartyWrite(ctx, tx, w)
			}
		}
		existingRaw, err := readPartyRaw(ctx, tx, partyID)
		if err != nil {
			return err
		}
		if existingRaw != "" && !w.dissolved(partyID) {
			outCode = code.LoginFail
			return execPartyWrite(ctx, tx, w)
		}
		created := &partyRecord{Leader: uid, Members: []partySeat{{UID: uid}}}
		w.set[partyID] = created
		w.bind[uid] = partyID
		w.onlineSet[uid] = nodeID
		out = created.view(partyID)
		outCode = code.OK
		return execPartyWrite(ctx, tx, w)
	})
	if err != nil {
		return PartyState{}, code.LoginFail
	}
	return out, outCode
}

// Invite 由队长邀请目标。新邀请覆盖同一目标的旧邀请。
// 先判断目标是否已在队（含宽限未过期，返回 40060），再判断有没有在线节点（没有则 40065）。
// 邀请自己返回 40063，非队长返回 40062。存储层不返回 40009。
func Invite(ctx context.Context, leader, target, inviteID, expireAt, now int64) int32 {
	ctx = ctxOrBG(ctx)
	if rdb == nil || leader < 1 || target < 1 || inviteID < 1 {
		return code.LoginFail
	}
	var outCode int32
	err := commitWatched(ctx, func() (*partySnap, error) {
		snap := newPartySnap()
		if err := snap.trackUser(ctx, leader); err != nil {
			return nil, err
		}
		if err := snap.trackUser(ctx, target); err != nil {
			return nil, err
		}
		if err := snap.trackOnline(ctx, target); err != nil {
			return nil, err
		}
		if err := snap.trackInvite(ctx, target); err != nil {
			return nil, err
		}
		return snap, nil
	}, func(tx *redis.Tx) error {
		outCode = code.OK
		w := newPartyWrite()
		pid, rec, err := loadUserParty(ctx, tx, leader)
		if err != nil {
			return err
		}
		var base *partyRecord
		if rec != nil {
			base, _ = activeAfterPrune(w, pid, rec, now)
		}
		if base == nil || !base.has(leader) || base.Leader != leader {
			outCode = code.PartyNotLeader
			return execPartyWrite(ctx, tx, w)
		}
		if target == leader {
			outCode = code.PartyInviteInvalid
			return execPartyWrite(ctx, tx, w)
		}
		tpid, trec, err := loadUserParty(ctx, tx, target)
		if err != nil {
			return err
		}
		var tbase *partyRecord
		if tpid == pid {
			tbase = base
		} else if trec != nil {
			tbase, _ = activeAfterPrune(w, tpid, trec, now)
		} else if tpid > 0 {
			w.unbind = append(w.unbind, target)
		}
		if tbase != nil && tbase.has(target) {
			outCode = code.PartyAlreadyIn
			return execPartyWrite(ctx, tx, w)
		}
		node, err := readOnline(ctx, tx, target)
		if err != nil {
			return err
		}
		if node == "" {
			outCode = code.PartyTargetOffline
			return execPartyWrite(ctx, tx, w)
		}
		w.inviteSet[target] = partyInviteRecord{
			InviteID: inviteID,
			PartyID:  pid,
			Leader:   leader,
			ExpireAt: expireAt,
		}
		return execPartyWrite(ctx, tx, w)
	})
	if err != nil {
		return code.LoginFail
	}
	return outCode
}

// Answer 应答邀请。接受时加到队尾；拒绝、邀请 id 不匹配，或 now 超过 expireAt，成员不变并返回 40063。
// 接受时队伍已有 4 人（含宽限未过期）返回 40064，并作废这条邀请。
func Answer(ctx context.Context, uid, inviteID int64, accept bool, now int64) (PartyState, int32) {
	ctx = ctxOrBG(ctx)
	if rdb == nil || uid < 1 || inviteID < 1 {
		return PartyState{}, code.LoginFail
	}
	var out PartyState
	var outCode int32
	err := commitWatched(ctx, func() (*partySnap, error) {
		snap := newPartySnap()
		if err := snap.trackInvite(ctx, uid); err != nil {
			return nil, err
		}
		inv, err := parseInvite(snap.invite[uid])
		if err != nil {
			return nil, err
		}
		if err := snap.trackUser(ctx, uid); err != nil {
			return nil, err
		}
		if err := snap.trackOnline(ctx, uid); err != nil {
			return nil, err
		}
		if inv != nil {
			if err := snap.trackMembers(ctx, inv.PartyID); err != nil {
				return nil, err
			}
		}
		return snap, nil
	}, func(tx *redis.Tx) error {
		out = PartyState{}
		outCode = code.PartyInviteInvalid
		w := newPartyWrite()
		raw, err := readInviteRaw(ctx, tx, uid)
		if err != nil {
			return err
		}
		inv, err := parseInvite(raw)
		if err != nil {
			return err
		}
		if inv == nil {
			return nil
		}
		pid := inv.PartyID
		partyRaw, err := readPartyRaw(ctx, tx, pid)
		if err != nil {
			return err
		}
		rec, err := parseParty(partyRaw)
		if err != nil {
			return err
		}
		var base *partyRecord
		if rec != nil {
			base, _ = activeAfterPrune(w, pid, rec, now)
		}
		view := PartyState{}
		if base != nil {
			view = base.view(pid)
		}
		if inv.InviteID != inviteID || now > inv.ExpireAt || !accept {
			out = view
			outCode = code.PartyInviteInvalid
			// 旧 inviteID 不能清掉覆盖后的新邀请。
			if inv.InviteID == inviteID {
				w.inviteDel = append(w.inviteDel, uid)
			}
			return execPartyWrite(ctx, tx, w)
		}
		if base == nil {
			w.inviteDel = append(w.inviteDel, uid)
			outCode = code.PartyInviteInvalid
			return execPartyWrite(ctx, tx, w)
		}
		inParty, err := userStillInParty(ctx, tx, w, uid, pid, base, now)
		if err != nil {
			return err
		}
		if inParty {
			out = view
			outCode = code.PartyAlreadyIn
			return execPartyWrite(ctx, tx, w)
		}
		node, err := readOnline(ctx, tx, uid)
		if err != nil {
			return err
		}
		if node == "" {
			out = view
			outCode = code.PartyTargetOffline
			return execPartyWrite(ctx, tx, w)
		}
		if len(base.Members) >= partyMaxMembers {
			w.inviteDel = append(w.inviteDel, uid)
			out = view
			outCode = code.PartyFull
			return execPartyWrite(ctx, tx, w)
		}
		updated := &partyRecord{
			Leader:  base.Leader,
			Members: append(append([]partySeat{}, base.Members...), partySeat{UID: uid}),
		}
		w.set[pid] = updated
		w.bind[uid] = pid
		w.inviteDel = append(w.inviteDel, uid)
		out = updated.view(pid)
		outCode = code.OK
		return execPartyWrite(ctx, tx, w)
	})
	if err != nil {
		return PartyState{}, code.LoginFail
	}
	return out, outCode
}

// userStillInParty 判断 uid 在摘掉过期宽限之后是否还占着某个队伍的名额。
func userStillInParty(ctx context.Context, tx redisGetter, w *partyWrite, uid, inviteParty int64, inviteBase *partyRecord, now int64) (bool, error) {
	if inviteBase != nil && inviteBase.has(uid) {
		return true, nil
	}
	pid, rec, err := loadUserParty(ctx, tx, uid)
	if err != nil {
		return false, err
	}
	if pid == 0 {
		return false, nil
	}
	if pid == inviteParty {
		return inviteBase != nil && inviteBase.has(uid), nil
	}
	if rec == nil {
		w.unbind = append(w.unbind, uid)
		return false, nil
	}
	active, _ := activeAfterPrune(w, pid, rec, now)
	return active != nil && active.has(uid), nil
}

// Leave 使 uid 离队。left 是离队者的空名单，rest 是留下的队伍。
// 队长离开后最早剩余成员接任；剩下不足 2 人则解散，两边 PartyID 都是 0。
// 不在队伍中返回 40061。不删除在线节点。
func Leave(ctx context.Context, uid, now int64) (PartyState, PartyState, int32) {
	ctx = ctxOrBG(ctx)
	if rdb == nil || uid < 1 {
		return PartyState{}, PartyState{}, code.LoginFail
	}
	var outLeft, outRest PartyState
	var outCode int32
	err := commitWatched(ctx, func() (*partySnap, error) {
		snap := newPartySnap()
		if err := snap.trackUser(ctx, uid); err != nil {
			return nil, err
		}
		return snap, nil
	}, func(tx *redis.Tx) error {
		outLeft = PartyState{}
		outRest = PartyState{}
		outCode = code.OK
		return leaveOrRemove(ctx, tx, uid, now, &outLeft, &outRest, &outCode)
	})
	if err != nil {
		return PartyState{}, PartyState{}, code.LoginFail
	}
	return outLeft, outRest, outCode
}

func leaveOrRemove(ctx context.Context, tx *redis.Tx, uid, now int64, left, rest *PartyState, outCode *int32) error {
	w := newPartyWrite()
	pid, rec, err := loadUserParty(ctx, tx, uid)
	if err != nil {
		return err
	}
	if pid == 0 || rec == nil {
		if pid > 0 {
			w.unbind = append(w.unbind, uid)
		}
		*outCode = code.PartyNotIn
		return execPartyWrite(ctx, tx, w)
	}
	base, _ := activeAfterPrune(w, pid, rec, now)
	if base == nil || !base.has(uid) {
		// 宽限在这次调用里到期，按离队成功处理；索引指向别人的队伍则视为本来就不在队。
		if rec.has(uid) {
			if base != nil {
				*rest = base.view(pid)
			}
			*outCode = code.OK
		} else {
			w.unbind = append(w.unbind, uid)
			*outCode = code.PartyNotIn
		}
		return execPartyWrite(ctx, tx, w)
	}
	updated := w.removeMember(pid, base, uid)
	if updated != nil {
		*rest = updated.view(pid)
	}
	*outCode = code.OK
	return execPartyWrite(ctx, tx, w)
}

// Kick 由队长把目标移出队伍。left 是被踢者的空名单，rest 是留下的队伍。
// 踢自己或目标不是本队成员返回 40063；调用者不是队长返回 40062。
// 宽限中的成员可以被踢，踢掉后宽限一并清除。
func Kick(ctx context.Context, leader, target, now int64) (PartyState, PartyState, int32) {
	ctx = ctxOrBG(ctx)
	if rdb == nil || leader < 1 || target < 1 {
		return PartyState{}, PartyState{}, code.LoginFail
	}
	var outLeft, outRest PartyState
	var outCode int32
	err := commitWatched(ctx, func() (*partySnap, error) {
		snap := newPartySnap()
		if err := snap.trackUser(ctx, leader); err != nil {
			return nil, err
		}
		if err := snap.trackUser(ctx, target); err != nil {
			return nil, err
		}
		return snap, nil
	}, func(tx *redis.Tx) error {
		outLeft = PartyState{}
		outRest = PartyState{}
		outCode = code.OK
		w := newPartyWrite()
		pid, rec, err := loadUserParty(ctx, tx, leader)
		if err != nil {
			return err
		}
		var base *partyRecord
		if rec != nil {
			base, _ = activeAfterPrune(w, pid, rec, now)
		}
		if base == nil || !base.has(leader) || base.Leader != leader {
			outCode = code.PartyNotLeader
			return execPartyWrite(ctx, tx, w)
		}
		if target == leader || !base.has(target) {
			outRest = base.view(pid)
			outCode = code.PartyInviteInvalid
			return execPartyWrite(ctx, tx, w)
		}
		updated := w.removeMember(pid, base, target)
		if updated != nil {
			outRest = updated.view(pid)
		}
		outCode = code.OK
		return execPartyWrite(ctx, tx, w)
	})
	if err != nil {
		return PartyState{}, PartyState{}, code.LoginFail
	}
	return outLeft, outRest, outCode
}

// PartyStateOf 返回 uid 当前队伍。不在队时 PartyID 为 0、code 为 0。
// 读取前先把已过宽限的成员按离队规则摘掉并写回。
func PartyStateOf(ctx context.Context, uid, now int64) (PartyState, int32) {
	ctx = ctxOrBG(ctx)
	if rdb == nil || uid < 1 {
		return PartyState{}, code.LoginFail
	}
	var out PartyState
	var outCode int32
	err := commitWatched(ctx, func() (*partySnap, error) {
		snap := newPartySnap()
		if err := snap.trackUser(ctx, uid); err != nil {
			return nil, err
		}
		return snap, nil
	}, func(tx *redis.Tx) error {
		out = PartyState{}
		outCode = code.OK
		w := newPartyWrite()
		pid, rec, err := loadUserParty(ctx, tx, uid)
		if err != nil {
			return err
		}
		if pid == 0 || rec == nil {
			if pid > 0 {
				w.unbind = append(w.unbind, uid)
			}
			return execPartyWrite(ctx, tx, w)
		}
		base, _ := activeAfterPrune(w, pid, rec, now)
		if base != nil && base.has(uid) {
			out = base.view(pid)
		} else if !rec.has(uid) {
			w.unbind = append(w.unbind, uid)
		}
		return execPartyWrite(ctx, tx, w)
	})
	if err != nil {
		return PartyState{}, code.LoginFail
	}
	return out, outCode
}

// BeginGrace 删除 uid 的在线节点。人还在队里时写下 graceUntil=now+60 秒，并留在原下标。
// 不在任何队伍，或摘掉过期成员后队伍已解散，仍删除在线节点，避免离线玩家被邀请；此时返回错误。
func BeginGrace(ctx context.Context, uid, now int64) error {
	ctx = ctxOrBG(ctx)
	if rdb == nil || uid < 1 {
		return fmt.Errorf("redis 不可用")
	}
	var outErr error
	err := commitWatched(ctx, func() (*partySnap, error) {
		snap := newPartySnap()
		if err := snap.trackUser(ctx, uid); err != nil {
			return nil, err
		}
		if err := snap.trackOnline(ctx, uid); err != nil {
			return nil, err
		}
		return snap, nil
	}, func(tx *redis.Tx) error {
		outErr = nil
		w := newPartyWrite()
		// 断线一律删在线键，包括不在队，以及过期摘人导致队伍先解散。
		w.onlineDel = append(w.onlineDel, uid)
		pid, rec, err := loadUserParty(ctx, tx, uid)
		if err != nil {
			return err
		}
		if pid == 0 || rec == nil {
			if pid > 0 {
				w.unbind = append(w.unbind, uid)
			}
			outErr = fmt.Errorf("不在队伍中")
			return execPartyWrite(ctx, tx, w)
		}
		base, _ := activeAfterPrune(w, pid, rec, now)
		if base == nil || !base.has(uid) {
			outErr = fmt.Errorf("不在队伍中")
			return execPartyWrite(ctx, tx, w)
		}
		members := append([]partySeat{}, base.Members...)
		for i := range members {
			if members[i].UID == uid {
				members[i].GraceUntil = now + partyGraceMS
			}
		}
		w.set[pid] = &partyRecord{Leader: base.Leader, Members: members}
		return execPartyWrite(ctx, tx, w)
	})
	if err != nil {
		return err
	}
	return outErr
}

// OnEnter 在进场时写回在线节点。
// 人还在名单里（宽限未过，或根本没有 graceUntil）则清掉宽限、回到原下标，bool 为 true。
// 宽限已过则按离队处理，bool 为 false，返回的名单为空。
func OnEnter(ctx context.Context, uid int64, nodeID string, now int64) (PartyState, bool, error) {
	ctx = ctxOrBG(ctx)
	if rdb == nil || uid < 1 {
		return PartyState{}, false, fmt.Errorf("redis 不可用")
	}
	var out PartyState
	var back bool
	err := commitWatched(ctx, func() (*partySnap, error) {
		snap := newPartySnap()
		if err := snap.trackUser(ctx, uid); err != nil {
			return nil, err
		}
		if err := snap.trackOnline(ctx, uid); err != nil {
			return nil, err
		}
		return snap, nil
	}, func(tx *redis.Tx) error {
		out = PartyState{}
		back = false
		w := newPartyWrite()
		w.onlineSet[uid] = nodeID
		pid, rec, err := loadUserParty(ctx, tx, uid)
		if err != nil {
			return err
		}
		if pid == 0 || rec == nil {
			if pid > 0 {
				w.unbind = append(w.unbind, uid)
			}
			return execPartyWrite(ctx, tx, w)
		}
		base, _ := activeAfterPrune(w, pid, rec, now)
		if base == nil || !base.has(uid) {
			return execPartyWrite(ctx, tx, w)
		}
		members := append([]partySeat{}, base.Members...)
		for i := range members {
			if members[i].UID == uid {
				members[i].GraceUntil = 0
			}
		}
		updated := &partyRecord{Leader: base.Leader, Members: members}
		w.set[pid] = updated
		out = updated.view(pid)
		back = true
		return execPartyWrite(ctx, tx, w)
	})
	if err != nil {
		return PartyState{}, false, err
	}
	return out, back, nil
}
