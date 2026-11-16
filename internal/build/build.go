// Package build holds the version stamped into the binary.
package build

import (
	"runtime/debug"
	"strings"
)

// Version and Date are set with -ldflags "-X ...". When they are not, the
// module version recorded by `go install pkg@version` is used.
var (
	Version = "DEV"
	Date    = ""
)

func init() {
	if Version != "DEV" {
		return
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		Version = strings.TrimPrefix(info.Main.Version, "v")
	}
}
