package loginroute

import (
	"fmt"
	"sync/atomic"
)

var seq atomic.Uint64

// ResetForTest 把轮询序号清零。仅测试调用。
func ResetForTest() {
	seq.Store(0)
}

// NextSessionPath 按请求轮询登录节点和工人。nodes 需已按 node id 排序。没有节点时返回空字符串。
func NextSessionPath(nodes []string, workers int) string {
	if len(nodes) == 0 {
		return ""
	}
	if workers < 1 {
		workers = 1
	}
	n := seq.Add(1) - 1
	node := nodes[int(n%uint64(len(nodes)))]
	worker := (n / uint64(len(nodes))) % uint64(workers)
	return fmt.Sprintf("%s.session.%d", node, worker)
}
