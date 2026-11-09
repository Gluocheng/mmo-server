package authcfg

import (
	"runtime"
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

// PasswordWorkersFrom 计算每个登录进程的密码工人数。
// 先按核数平分到登录节点，至少 2 个；再留给 session 工人和其余数据库调用 4 条连接。
func PasswordWorkersFrom(cpu, nodes, openConns, sessionWorkers int) int {
	if nodes < 1 {
		nodes = 1
	}
	if cpu < 1 {
		cpu = 1
	}
	n := cpu / nodes
	if n < 2 {
		n = 2
	}
	if openConns < 1 {
		openConns = 20
	}
	if sessionWorkers < 1 {
		sessionWorkers = 8
	}
	room := openConns - sessionWorkers - 4
	if room < 2 {
		room = 2
	}
	if n > room {
		n = room
	}
	return n
}

// PasswordWorkers 返回本进程应启动的密码工人数。节点数读 profile，核数读本机。
func PasswordWorkers() int {
	openConns := 20
	if cfg := cprofile.GetConfig("mysql"); cfg != nil {
		if v := cfg.GetInt("max_open_conns", 20); v > 0 {
			openConns = v
		}
	}
	return PasswordWorkersFrom(runtime.NumCPU(), len(EnabledLoginNodeIDs()), openConns, SessionWorkers())
}

// EnabledLoginNodeIDs 返回 profile 里启用的登录节点，按 node id 升序。读不到时退回 login-1。
func EnabledLoginNodeIDs() []string {
	raw := cprofile.GetConfig("node").GetConfig("login")
	if raw == nil || raw.LastError() != nil || raw.Size() < 1 {
		return []string{"login-1"}
	}
	ids := make([]string, 0, raw.Size())
	for i := 0; i < raw.Size(); i++ {
		item := raw.GetConfig(i)
		if item == nil || item.LastError() != nil || !item.GetBool("enable") {
			continue
		}
		id := strings.TrimSpace(item.GetString("node_id"))
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
