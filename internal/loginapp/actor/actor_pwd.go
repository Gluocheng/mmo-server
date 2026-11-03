package actor

import (
	"errors"
	"strconv"
	"strings"

	cfacade "github.com/cherry-game/cherry/facade"
	clog "github.com/cherry-game/cherry/logger"
	cactor "github.com/cherry-game/cherry/net/actor"
	cnats "github.com/cherry-game/cherry/net/nats"
	cproto "github.com/cherry-game/cherry/net/proto"
	"github.com/example/mmo-server/internal/authcfg"
	"github.com/example/mmo-server/internal/code"
	"github.com/example/mmo-server/internal/loginapp/admit"
	"github.com/example/mmo-server/internal/persistence"
	"github.com/example/mmo-server/internal/protocol"
	"google.golang.org/protobuf/proto"
)

// ActorPwd 登录服密码哈希父 Actor，路径 {node}.pwd。只负责限流和分发，不算哈希。
type ActorPwd struct {
	cactor.Base
	gate *admit.Gate
}

func (p *ActorPwd) AliasID() string {
	return "pwd"
}

// OnInit 按本机核数和数据库连接池建好密码工人。
func (p *ActorPwd) OnInit() {
	n := authcfg.PasswordWorkers()
	p.gate = admit.New(n)
	for i := 0; i < n; i++ {
		if _, err := p.Child().Create(strconv.Itoa(i), &ActorPwdWorker{id: i, gate: p.gate}); err != nil {
			clog.Warnf("create login password worker %d: %v", i, err)
		}
	}
}

// OnRemoteReceived 签发先占名额。目标工人满了就换下一个，都满则马上回复登录繁忙，不进入工人邮箱。
func (p *ActorPwd) OnRemoteReceived(m *cfacade.Message) (bool, bool) {
	if m == nil || m.FuncName != "issueToken" || m.TargetPath() == nil || m.TargetPath().ChildID == "" {
		return true, false
	}
	start, err := strconv.Atoi(m.TargetPath().ChildID)
	if err != nil {
		start = 0
	}
	id, ok := p.gate.AcquireFrom(start)
	if !ok {
		replyCode(m, code.LoginBusy)
		return false, false
	}
	child, found := p.Child().Get(strconv.Itoa(id))
	if !found {
		p.gate.Release(id)
		replyCode(m, code.LoginBusy)
		return false, false
	}
	child.PostRemote(m)
	return false, false
}

// ActorPwdWorker 只处理签发。哈希期间不占用 session 工人。
type ActorPwdWorker struct {
	cactor.Base
	id   int
	gate *admit.Gate
}

func (p *ActorPwdWorker) OnInit() {
	p.Remote().Register("issueToken", p.issueToken)
}

func (p *ActorPwdWorker) issueToken(req *protocol.IssueTokenRequest) (*protocol.IssueTokenResponse, int32) {
	defer p.gate.Release(p.id)
	if req == nil || strings.TrimSpace(req.Nickname) == "" || strings.TrimSpace(req.Password) == "" {
		return nil, code.LoginFail
	}
	if c := rejectIfMaintenance(); c != code.OK {
		return nil, c
	}
	if blocked, err := persistence.IsLoginBlocked(req.ClientIp, req.Nickname); err == nil && blocked {
		return nil, code.LoginRateLimited
	}

	deviceID := strings.TrimSpace(req.DeviceId)
	if deviceID == "" {
		return nil, code.DeviceIDRequired
	}

	uid, err := persistence.LoginOrCreateAccount(req.Nickname, req.Password)
	if err != nil || uid < 1 {
		clog.Warnf("issueToken account fail nickname=%s uid=%d err=%v", req.Nickname, uid, err)
		_ = persistence.RecordLoginFailure(req.ClientIp, req.Nickname)
		if errors.Is(err, persistence.ErrInvalidPassword) {
			return nil, code.InvalidPassword
		}
		return nil, code.LoginFail
	}
	if c := rejectIfBanned(uid); c != code.OK {
		return nil, c
	}
	_ = persistence.ClearLoginFailure(req.ClientIp, req.Nickname)
	accessToken, accessExpireAt, refreshToken, refreshExpireAt, err := persistence.IssueTokenPair(uid, deviceID)
	if err != nil {
		clog.Warnf("issueToken pair fail uid=%d err=%v", uid, err)
		return nil, code.LoginFail
	}
	return &protocol.IssueTokenResponse{
		Uid:             uid,
		AccessToken:     accessToken,
		AccessExpireAt:  accessExpireAt,
		RefreshToken:    refreshToken,
		RefreshExpireAt: refreshExpireAt,
	}, code.OK
}

func replyCode(m *cfacade.Message, c int32) {
	if m == nil || m.Reply == "" {
		return
	}
	body, err := proto.Marshal(&cproto.Response{Code: c})
	if err != nil {
		clog.Warnf("reply login code=%d err=%v", c, err)
		return
	}
	if err = cnats.ReplySync(m.ReqID, m.Reply, body); err != nil {
		clog.Warnf("reply login code=%d err=%v", c, err)
	}
}
