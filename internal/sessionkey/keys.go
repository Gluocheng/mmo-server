package sessionkey

// Session 中存储的键（与 Cherry pomelo session 一致）
const (
	ServerID = "server_id"
	Nickname = "nickname"
	PlayerID = "player_id"
	// 兼容旧键 token，值等同 access_token
	Token        = "token"
	AccessToken  = "access_token"
	RefreshToken = "refresh_token"
	DeviceID     = "device_id"
)
