//go:build windows

package config

import (
	"errors"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

const windowsFileRetryLimit = 100

func readConfigFile(path string) ([]byte, error) {
	for attempt := 0; ; attempt++ {
		payload, err := os.ReadFile(path)
		if err == nil || !retryWindowsFileOperation(err, attempt) {
			return payload, err
		}
	}
}

func replaceFile(source string, target string) error {
	sourcePath, err := windows.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	targetPath, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	for attempt := 0; ; attempt++ {
		err = windows.MoveFileEx(
			sourcePath,
			targetPath,
			windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH,
		)
		if err == nil {
			return nil
		}
		if !retryWindowsFileOperation(err, attempt) {
			return err
		}
	}
}

func retryWindowsFileOperation(err error, attempt int) bool {
	if attempt+1 >= windowsFileRetryLimit ||
		(!errors.Is(err, windows.ERROR_ACCESS_DENIED) &&
			!errors.Is(err, windows.ERROR_SHARING_VIOLATION)) {
		return false
	}
	time.Sleep(5 * time.Millisecond)
	return true
}
