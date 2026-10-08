package authcfg

import (
	"sort"
	"strings"

	cprofile "github.com/cherry-game/cherry/profile"
)

// SessionWorkers 返回每个登录节点的 session 工人数。未配置时为 8，上限 16，以免超过单进程 MySQL 连接池。
func SessionWorkers() int {
	n := 8
	if cfg := cprofile.GetConfig("auth"); cfg != nil {
		if v := cfg.GetInt("session_workers", 8); v > 0 {
			n = v
		}
	}
	if n < 1 {
		n = 1
	}
	if n > 16 {
		n = 16
	}
	return n
}

// EnabledLoginNodeIDs 返回 profile 里启用的登录节点，按 node id 升序。读不到时退回 login-1。
func EnabledLoginNodeIDs() []string {
	raw := cprofile.GetConfig("node", "login")
	if raw == nil || raw.Size() < 1 {
		return []string{"login-1"}
	}
	ids := make([]string, 0, raw.Size())
	for i := 0; i < raw.Size(); i++ {
		item := raw.Get(i)
		if item == nil || item.LastError() != nil || !item.Get("enable").ToBool() {
			continue
		}
		id := strings.TrimSpace(item.Get("node_id").ToString())
		if id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return []string{"login-1"}
	}
	sort.Strings(ids)
	return ids
}
