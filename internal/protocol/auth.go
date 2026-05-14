package protocol

// IssueTokenRequest 用户名密码换取 token
type IssueTokenRequest struct {
	Nickname string `json:"nickname"`
	Password string `json:"password"`
	DeviceID string `json:"deviceId"`
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
	DeviceID    string `json:"deviceId"`
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
