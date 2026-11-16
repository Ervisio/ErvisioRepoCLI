//go:build !darwin && !linux

package iostreams

import (
	"errors"
	"os"
)

func termWidth(*os.File) int { return 0 }

func disableEcho(*os.File) (func(), error) {
	return nil, errors.New("disabling echo is not supported on this platform")
}
