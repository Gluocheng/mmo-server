package gateentry

import "sync"

// Gate 是一个已启用的网关后端。
type Gate struct {
	ID      string
	Address string
	Active  int
	Max     int
}

// Balancer 按当前连接数把新连接分到未满的网关。
type Balancer struct {
	mu    sync.Mutex
	gates []Gate
}

// NewBalancer 复制网关列表。Max 小于 1 时按 1。
func NewBalancer(gates []Gate) *Balancer {
	cp := make([]Gate, len(gates))
	for i, g := range gates {
		if g.Max < 1 {
			g.Max = 1
		}
		cp[i] = g
	}
	return &Balancer{gates: cp}
}

// Acquire 选中当前连接最少且未满的网关，并把它的计数加一。没有可用网关时 ok 为 false。
func (b *Balancer) Acquire() (addr string, release func(), ok bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	best := -1
	for i := range b.gates {
		if b.gates[i].Active >= b.gates[i].Max {
			continue
		}
		if best < 0 || b.gates[i].Active < b.gates[best].Active {
			best = i
		}
	}
	if best < 0 {
		return "", nil, false
	}
	b.gates[best].Active++
	idx := best
	return b.gates[idx].Address, func() {
		b.mu.Lock()
		if b.gates[idx].Active > 0 {
			b.gates[idx].Active--
		}
		b.mu.Unlock()
	}, true
}
