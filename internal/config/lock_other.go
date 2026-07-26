//go:build !windows && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris

package config

import (
	"fmt"
	"runtime"
)

// The remaining Go targets do not expose a process-owned advisory lock through
// the standard library. Fail closed instead of risking concurrent config
// updates or leaving an unrecoverable file-creation lock after a crash.
func tryAcquireConfigFileLock(_ string) (func() error, bool, error) {
	return nil, false, fmt.Errorf(
		"cross-process config locking is unsupported on %s",
		runtime.GOOS,
	)
}
