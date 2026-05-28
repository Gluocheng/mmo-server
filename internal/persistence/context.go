package persistence

import (
	"context"
	"time"
)

const defaultOpTimeout = 2 * time.Second

func opContext(parent context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}

	timeout := cfg.opTimeout
	if timeout <= 0 {
		timeout = defaultOpTimeout
	}

	return context.WithTimeout(parent, timeout)
}
