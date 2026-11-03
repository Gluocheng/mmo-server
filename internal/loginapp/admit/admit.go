package admit

import "sync/atomic"

// QueueLimit 每个密码工人同时占住的名额，含正在算哈希的那一条。
const QueueLimit int32 = 32

// Gate 按工人计数。满了的工人不再接收，调用方改试下一个。
type Gate struct {
	pending []atomic.Int32
}

// New 建立 n 个工人的计数。n 小于 1 时按 1 个。
func New(n int) *Gate {
	if n < 1 {
		n = 1
	}
	return &Gate{pending: make([]atomic.Int32, n)}
}

// Len 返回工人数。
func (g *Gate) Len() int {
	if g == nil {
		return 0
	}
	return len(g.pending)
}

// AcquireFrom 从 start 起轮询，占住第一个未满的工人。都满时 ok 为 false。
func (g *Gate) AcquireFrom(start int) (id int, ok bool) {
	if g == nil || len(g.pending) == 0 {
		return 0, false
	}
	n := len(g.pending)
	if start < 0 {
		start = 0
	}
	start %= n
	for i := 0; i < n; i++ {
		id = (start + i) % n
		for {
			cur := g.pending[id].Load()
			if cur >= QueueLimit {
				break
			}
			if g.pending[id].CompareAndSwap(cur, cur+1) {
				return id, true
			}
		}
	}
	return 0, false
}

// Release 归还工人名额。重复归还不会降到 0 以下。
func (g *Gate) Release(id int) {
	if g == nil || id < 0 || id >= len(g.pending) {
		return
	}
	for {
		cur := g.pending[id].Load()
		if cur <= 0 {
			return
		}
		if g.pending[id].CompareAndSwap(cur, cur-1) {
			return
		}
	}
}

// Pending 返回该工人当前名额，测试用。
func (g *Gate) Pending(id int) int32 {
	if g == nil || id < 0 || id >= len(g.pending) {
		return 0
	}
	return g.pending[id].Load()
}
