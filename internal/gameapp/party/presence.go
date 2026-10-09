package party

import "sync"

// 本机已进场玩家的网关路径。推送只发给这张表里的 uid。
var (
	presenceMu sync.RWMutex
	presence   = map[int64]string{}
)

// Bind 记下 uid 的网关路径。进场成功并写完 session 后调用。
// uid 无效或路径为空时忽略。
func Bind(uid int64, agentPath string) {
	if uid < 1 || agentPath == "" {
		return
	}
	presenceMu.Lock()
	presence[uid] = agentPath
	presenceMu.Unlock()
}

// Unbind 去掉 uid 的本机进场记录。断线时调用。
func Unbind(uid int64) {
	presenceMu.Lock()
	delete(presence, uid)
	presenceMu.Unlock()
}

// Path 返回本机已进场 uid 的网关路径。未 Bind 时 ok 为 false。
func Path(uid int64) (string, bool) {
	presenceMu.RLock()
	defer presenceMu.RUnlock()
	path, ok := presence[uid]
	if !ok || path == "" {
		return "", false
	}
	return path, true
}
