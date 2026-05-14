package actor

import (
	"net"
	"strings"

	cstring "github.com/cherry-game/cherry/extend/string"
	cfacade "github.com/cherry-game/cherry/facade"
	cprofile "github.com/cherry-game/cherry/profile"
	clog "github.com/cherry-game/cherry/logger"
	cactor "github.com/cherry-game/cherry/net/actor"
	"github.com/cherry-game/cherry/net/parser/pomelo"
	pmessage "github.com/cherry-game/cherry/net/parser/pomelo/message"
	cproto "github.com/cherry-game/cherry/net/proto"
	"github.com/example/mmo-server/internal/code"
	"github.com/example/mmo-server/internal/persistence"
	"github.com/example/mmo-server/internal/protocol"
	"github.com/example/mmo-server/internal/sessionkey"
)

const (
	loginRoute        = "gate.user.login"
	issueTokenRoute   = "gate.user.issueToken"
	refreshTokenRoute = "gate.user.refreshToken"

	policyKickOld    = "kick_old"
	policyCoexist    = "coexist"
	policyDeviceLimit = "device_limit"
)

var beforeEnterRoutes = map[string]struct{}{
	"game.player.select": {},
	"game.player.create": {},
	"game.player.enter":  {},
}

var notLoginKick = &struct {
	Code int32 `json:"code"`
}{Code: code.PlayerDenyLogin}

func loginTargetPath(app cfacade.IApplication) string {
	list := app.Discovery().ListByType("login")
	if len(list) < 1 {
		return ""
	}
	return cstring.ToString(list[0].GetNodeID()) + ".session"
}

// AgentActor 每个连接一个子 Actor
type AgentActor struct {
	cactor.Base
}

func (p *AgentActor) OnInit() {
	p.Local().Register("issueToken", p.issueToken)
	p.Local().Register("login", p.login)
	p.Local().Register("refreshToken", p.refreshToken)
	p.Local().Register("logout", p.logout)
	p.Remote().Register("setSession", p.setSession)
}

func (p *AgentActor) issueToken(session *cproto.Session, req *protocol.IssueTokenRequest) {
	if req == nil {
		pomelo.ResponseCode(p, session.AgentPath, session.Sid, session.GetMID(), code.LoginFail)
		return
	}
	agent, ok := pomelo.GetAgentWithSID(p.ActorID())
	if !ok {
		pomelo.ResponseCode(p, session.AgentPath, session.Sid, session.GetMID(), code.LoginFail)
		return
	}
	target := loginTargetPath(p.App())
	if target == "" {
		pomelo.ResponseCode(p, session.AgentPath, session.Sid, session.GetMID(), code.LoginRPCFail)
		return
	}
	rsp := &protocol.IssueTokenResponse{}
	reqCopy := *req
	reqCopy.ClientIP = clientIPFromRemoteAddr(agent.RemoteAddr())
	errCode := p.App().ActorSystem().CallWait(p.PathString(), target, "issueToken", &reqCopy, rsp)
	if code.IsFail(errCode) {
		pomelo.ResponseCode(p, session.AgentPath, session.Sid, session.GetMID(), mapAuthCode(errCode, code.LoginRPCFail))
		return
	}
	if rsp.AccessToken != "" {
		if agent, ok := pomelo.GetAgentWithSID(p.ActorID()); ok {
			agent.Session().Set(sessionkey.AccessToken, rsp.AccessToken)
			agent.Session().Set(sessionkey.Token, rsp.AccessToken)
			agent.Session().Set(sessionkey.RefreshToken, rsp.RefreshToken)
		}
	}
	pomelo.Response(p, session.AgentPath, session.Sid, session.GetMID(), rsp)
}

func clientIPFromRemoteAddr(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return host
}

func (p *AgentActor) login(session *cproto.Session, req *protocol.TokenLoginRequest) {
	agent, ok := pomelo.GetAgentWithSID(p.ActorID())
	if !ok {
		return
	}
	if req == nil {
		pomelo.ResponseCode(p, session.AgentPath, session.Sid, session.GetMID(), code.LoginFail)
		return
	}
	deviceID := strings.TrimSpace(req.DeviceID)
	if deviceID == "" {
		pomelo.ResponseCode(p, session.AgentPath, session.Sid, session.GetMID(), code.DeviceIDRequired)
		return
	}
	accessToken := req.AccessToken
	if accessToken == "" {
		accessToken = req.Token
	}
	if accessToken == "" {
		pomelo.ResponseCode(p, session.AgentPath, session.Sid, session.GetMID(), code.LoginFail)
		return
	}
	target := loginTargetPath(p.App())
	if target == "" {
		clog.Error("no login node in discovery")
		pomelo.ResponseCode(p, session.AgentPath, session.Sid, session.GetMID(), code.LoginRPCFail)
		return
	}
	rsp := &protocol.TokenLoginResponse{}
	authReq := &protocol.TokenLoginRequest{
		AccessToken: accessToken,
		ServerID:    req.ServerID,
		DeviceID:    deviceID,
	}
	errCode := p.App().ActorSystem().CallWait(p.PathString(), target, "authToken", authReq, rsp)
	if code.IsFail(errCode) {
		pomelo.ResponseCode(p, session.AgentPath, session.Sid, session.GetMID(), mapAuthCode(errCode, code.LoginRPCFail))
		return
	}
	if rsp.UID < 1 {
		pomelo.ResponseCode(p, session.AgentPath, session.Sid, session.GetMID(), code.LoginFail)
		return
	}

	policy, maxDevices := sessionPolicyConfig()
	if policy == policyDeviceLimit {
		allowed, err := persistence.RegisterDeviceSession(rsp.UID, deviceID, maxDevices)
		if err != nil {
			clog.Warnf("register device session fail uid=%d device=%s err=%v", rsp.UID, deviceID, err)
			pomelo.ResponseCode(p, session.AgentPath, session.Sid, session.GetMID(), code.LoginFail)
			return
		}
		if !allowed {
			pomelo.ResponseCode(p, session.AgentPath, session.Sid, session.GetMID(), code.DeviceLimitReached)
			return
		}
	}

	oldAgent, err := pomelo.Bind(session.Sid, rsp.UID)
	if err != nil {
		clog.Warnf("bind uid fail: %v", err)
		pomelo.ResponseCode(p, session.AgentPath, session.Sid, session.GetMID(), code.LoginFail)
		return
	}
	if policy == policyKickOld {
		if oldAgent != nil {
			oldAgent.Kick(notLoginKick, true)
		}
		p.kickUIDOnOtherGates(rsp.UID)
	}
	agent.Session().Set(sessionkey.ServerID, cstring.ToString(req.ServerID))
	agent.Session().Set(sessionkey.AccessToken, accessToken)
	agent.Session().Set(sessionkey.Token, accessToken)
	agent.Session().Set(sessionkey.DeviceID, deviceID)
	pomelo.Response(p, session.AgentPath, session.Sid, session.GetMID(), rsp)
}

func (p *AgentActor) refreshToken(session *cproto.Session, req *protocol.RefreshTokenRequest) {
	agent, ok := pomelo.GetAgentWithSID(p.ActorID())
	if !ok {
		return
	}
	if req == nil || req.RefreshToken == "" {
		pomelo.ResponseCode(p, session.AgentPath, session.Sid, session.GetMID(), code.LoginFail)
		return
	}
	target := loginTargetPath(p.App())
	if target == "" {
		pomelo.ResponseCode(p, session.AgentPath, session.Sid, session.GetMID(), code.LoginRPCFail)
		return
	}
	rsp := &protocol.RefreshTokenResponse{}
	errCode := p.App().ActorSystem().CallWait(p.PathString(), target, "refreshToken", req, rsp)
	if code.IsFail(errCode) || rsp.AccessToken == "" || rsp.RefreshToken == "" {
		if code.IsFail(errCode) {
			pomelo.ResponseCode(p, session.AgentPath, session.Sid, session.GetMID(), mapAuthCode(errCode, code.LoginFail))
		} else {
			pomelo.ResponseCode(p, session.AgentPath, session.Sid, session.GetMID(), code.LoginFail)
		}
		return
	}
	agent.Session().Set(sessionkey.AccessToken, rsp.AccessToken)
	agent.Session().Set(sessionkey.Token, rsp.AccessToken)
	agent.Session().Set(sessionkey.RefreshToken, rsp.RefreshToken)
	pomelo.Response(p, session.AgentPath, session.Sid, session.GetMID(), rsp)
}

func (p *AgentActor) logout(session *cproto.Session, req *protocol.LogoutRequest) {
	agent, ok := pomelo.GetAgentWithSID(p.ActorID())
	if !ok {
		return
	}
	if !session.IsBind() || req == nil {
		pomelo.ResponseCode(p, session.AgentPath, session.Sid, session.GetMID(), code.LoginFail)
		return
	}
	accessToken := req.AccessToken
	if accessToken == "" {
		accessToken = req.Token
	}
	if accessToken == "" {
		accessToken = agent.Session().GetString(sessionkey.AccessToken)
	}
	refreshToken := req.RefreshToken
	if refreshToken == "" {
		refreshToken = agent.Session().GetString(sessionkey.RefreshToken)
	}
	if accessToken == "" && refreshToken == "" {
		pomelo.ResponseCode(p, session.AgentPath, session.Sid, session.GetMID(), code.LoginFail)
		return
	}
	target := loginTargetPath(p.App())
	if target == "" {
		pomelo.ResponseCode(p, session.AgentPath, session.Sid, session.GetMID(), code.LoginRPCFail)
		return
	}
	rsp := &protocol.LogoutResponse{}
	logoutReq := &protocol.LogoutRequest{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}
	errCode := p.App().ActorSystem().CallWait(p.PathString(), target, "logout", logoutReq, rsp)
	if code.IsFail(errCode) || !rsp.OK {
		if code.IsFail(errCode) {
			pomelo.ResponseCode(p, session.AgentPath, session.Sid, session.GetMID(), mapAuthCode(errCode, code.LoginFail))
		} else {
			pomelo.ResponseCode(p, session.AgentPath, session.Sid, session.GetMID(), code.LoginFail)
		}
		return
	}

	if session.Uid > 0 {
		_ = persistence.RemoveDeviceSession(session.Uid, agent.Session().GetString(sessionkey.DeviceID))
	}
	agent.Unbind()
	agent.Session().Remove(sessionkey.Token)
	agent.Session().Remove(sessionkey.AccessToken)
	agent.Session().Remove(sessionkey.RefreshToken)
	agent.Session().Remove(sessionkey.DeviceID)
	agent.Session().Remove(sessionkey.PlayerID)
	agent.Session().Remove(sessionkey.ServerID)
	pomelo.Response(p, session.AgentPath, session.Sid, session.GetMID(), rsp)
}

func (p *AgentActor) setSession(req *protocol.StringKeyValue) {
	if req == nil || req.Key == "" {
		return
	}
	agent, ok := pomelo.GetAgentWithSID(p.ActorID())
	if !ok {
		return
	}
	agent.Session().Set(req.Key, req.Value)
}

func (p *AgentActor) OnSessionClose(agent *pomelo.Agent) {
	session := agent.Session()
	if session.Uid > 0 {
		_ = persistence.RemoveDeviceSession(session.Uid, session.GetString(sessionkey.DeviceID))
	}
	serverID := session.GetString(sessionkey.ServerID)
	if serverID == "" {
		p.Exit()
		return
	}
	childID := cstring.ToString(session.Uid)
	if childID != "" {
		target := cfacade.NewChildPath(serverID, "player", childID)
		p.Call(target, "sessionClose", nil)
	}
	p.Exit()
}

// OnPomeloDataRoute 网关消息路由
func OnPomeloDataRoute(agent *pomelo.Agent, route *pmessage.Route, msg *pmessage.Message) {
	session := pomelo.BuildSession(agent, msg)
	if !session.IsBind() && msg.Route != loginRoute && msg.Route != issueTokenRoute && msg.Route != refreshTokenRoute {
		agent.Kick(notLoginKick, true)
		return
	}
	if agent.NodeType() == route.NodeType() {
		targetPath := cfacade.NewChildPath(agent.NodeID(), route.HandleName(), session.Sid)
		pomelo.LocalDataRoute(agent, session, route, msg, targetPath)
		return
	}
	gameNodeRoute(agent, session, route, msg)
}

func gameNodeRoute(agent *pomelo.Agent, session *cproto.Session, route *pmessage.Route, msg *pmessage.Message) {
	if !session.IsBind() {
		return
	}
	if !session.Contains(sessionkey.PlayerID) {
		if _, ok := beforeEnterRoutes[msg.Route]; !ok {
			agent.Kick(notLoginKick, true)
			return
		}
	}
	serverID := session.GetString(sessionkey.ServerID)
	if serverID == "" {
		return
	}
	childID := cstring.ToString(session.Uid)
	targetPath := cfacade.NewChildPath(serverID, route.HandleName(), childID)
	if err := pomelo.ClusterLocalDataRoute(agent, session, route, msg, serverID, targetPath); err != nil {
		clog.Warnf("cluster route err: %v", err)
	}
}

func mapAuthCode(remoteCode int32, fallback int32) int32 {
	// 业务码约定从 40000 开始；框架码回退为 fallback
	if remoteCode >= 40000 {
		return remoteCode
	}
	return fallback
}

func sessionPolicyConfig() (policy string, maxDevices int) {
	authCfg := cprofile.GetConfig("auth")
	policy = strings.TrimSpace(authCfg.GetString("session_policy", policyKickOld))
	maxDevices = authCfg.GetInt("max_devices_per_uid", 2)
	if maxDevices < 1 {
		maxDevices = 1
	}
	return policy, maxDevices
}

func (p *AgentActor) kickUIDOnOtherGates(uid int64) {
	rsp := &cproto.PomeloKick{
		Uid:    uid,
		Reason: []byte{},
		Close:  true,
	}
	members := p.App().Discovery().ListByType(p.App().NodeType(), p.App().NodeID())
	for _, member := range members {
		target := cfacade.NewPath(member.GetNodeID(), "user")
		p.Call(target, pomelo.KickFuncName, rsp)
	}
}
