package authcfg

import (
	"sort"
	"strings"

	cprofile "github.com/cherry-game/cherry/profile"
)

// maxPasswordWorkers 是每个登录进程密码工人数的上限，与 session 工人上限一致，避免单进程开过多连接。
const maxPasswordWorkers = 16

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
// 按可用数据库连接数决定，留给 session 工人和其余数据库调用 4 条连接，上限 16。
// 不再按核数平分：bcrypt 是纯 CPU 活，连接够就该多开工人，
// 否则 2000 人同时签发的吞吐被工人数卡死（实测每请求约 196ms，20 秒窗口需要 ≥14 并发）。
func PasswordWorkersFrom(openConns, sessionWorkers int) int {
	if openConns < 1 {
		openConns = 20
	}
	if sessionWorkers < 1 {
		sessionWorkers = 8
	}
	n := openConns - sessionWorkers - 4
	if n < 2 {
		n = 2
	}
	if n > maxPasswordWorkers {
		n = maxPasswordWorkers
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
	return PasswordWorkersFrom(openConns, SessionWorkers())
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
