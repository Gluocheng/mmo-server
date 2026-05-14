package protocol

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
