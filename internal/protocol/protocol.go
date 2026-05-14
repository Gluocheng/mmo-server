package protocol

// IssueTokenRequest 用户名密码换取 token
type IssueTokenRequest struct {
	Nickname string `json:"nickname"`
	Password string `json:"password"`
	// ClientIP 由网关注入，客户端无需传
	ClientIP string `json:"clientIp"`
}

type IssueTokenResponse struct {
	UID             int64  `json:"uid"`
	AccessToken     string `json:"accessToken"`
	AccessExpireAt  int64  `json:"accessExpireAt"`
	RefreshToken    string `json:"refreshToken"`
	RefreshExpireAt int64  `json:"refreshExpireAt"`
}

// TokenLoginRequest 使用 token 登录网关
type TokenLoginRequest struct {
	// 兼容旧字段 token；优先 accessToken
	AccessToken string `json:"accessToken"`
	Token       string `json:"token"`
	ServerID    int32  `json:"serverId"`
}

type TokenLoginResponse struct {
	UID int64 `json:"uid"`
}

type LogoutRequest struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	// 兼容旧字段 token -> accessToken
	Token string `json:"token"`
}

type LogoutResponse struct {
	OK bool `json:"ok"`
}

type RefreshTokenRequest struct {
	RefreshToken string `json:"refreshToken"`
}

type RefreshTokenResponse struct {
	AccessToken     string `json:"accessToken"`
	AccessExpireAt  int64  `json:"accessExpireAt"`
	RefreshToken    string `json:"refreshToken"`
	RefreshExpireAt int64  `json:"refreshExpireAt"`
}

// StringKeyValue 用于跨节点同步网关 session 字段
type StringKeyValue struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// PlayerInfo 演示角色信息（单服单角色）
type PlayerInfo struct {
	PlayerID int64  `json:"playerId"`
	Name     string `json:"name"`
}

type None struct{}

type PlayerSelectResponse struct {
	List []PlayerInfo `json:"list"`
}

type PlayerCreateRequest struct {
	Name string `json:"name"`
}

type PlayerCreateResponse struct {
	Player PlayerInfo `json:"player"`
}

// EnterGameRequest 进入场景（演示无 DB，直接进入默认场景）
type EnterGameRequest struct {
	PlayerID int64 `json:"playerId"`
	SceneID  int32 `json:"sceneId"`
}

// EnterGameResponse 进入场景回包
type EnterGameResponse struct {
	SceneID int32   `json:"sceneId"`
	Players []int64 `json:"players"`
}

// MoveRequest 移动
type MoveRequest struct {
	X float32 `json:"x"`
	Y float32 `json:"y"`
	Z float32 `json:"z"`
}

// MoveBroadcast 同场景广播
type MoveBroadcast struct {
	UID int64   `json:"uid"`
	X   float32 `json:"x"`
	Y   float32 `json:"y"`
	Z   float32 `json:"z"`
}
