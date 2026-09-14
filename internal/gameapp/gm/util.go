package gm

import (
	"errors"

	"github.com/example/mmo-server/internal/code"
	"github.com/example/mmo-server/internal/persistence"
)

func bagErrorCode(err error) int32 {
	switch {
	case errors.Is(err, persistence.ErrBagInvalid):
		return code.BagItemInvalid
	case errors.Is(err, persistence.ErrBagNotEnough):
		return code.BagItemNotEnough
	case errors.Is(err, persistence.ErrBagSlotInvalid):
		return code.BagSlotInvalid
	case errors.Is(err, persistence.ErrBagFull):
		return code.BagFull
	case errors.Is(err, persistence.ErrItemNotFound):
		return code.ItemNotFound
	case errors.Is(err, persistence.ErrBagTypeMismatch):
		return code.BagTypeMismatch
	default:
		return code.BagLoadFail
	}
}

func exactlyOne(flags ...bool) bool {
	n := 0
	for _, f := range flags {
		if f {
			n++
		}
	}
	return n == 1
}
