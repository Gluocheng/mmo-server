package protocol

// PlayerInfo 演示角色信息（单服单角色）
type PlayerInfo struct {
	PlayerID int64  `json:"playerId"`
	Name     string `json:"name"`
}

type PlayerSelectResponse struct {
	List []PlayerInfo `json:"list"`
}

type PlayerCreateRequest struct {
	Name string `json:"name"`
}

type PlayerCreateResponse struct {
	Player PlayerInfo `json:"player"`
}
