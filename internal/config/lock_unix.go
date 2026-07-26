//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package config

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func tryAcquireConfigFileLock(path string) (func() error, bool, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, false, err
	}
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return func() error {
		return errors.Join(
			unix.Flock(int(file.Fd()), unix.LOCK_UN),
			file.Close(),
		)
	}, true, nil
}
