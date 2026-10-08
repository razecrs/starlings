//go:build !windows && !linux

package sysmedia

import (
	"errors"

	"github.com/razecrs/starlings"
)

func open() (starlings.StarlogMediaReader, error) {
	return nil, errors.New("sysmedia: system media detection is unavailable on this platform")
}
