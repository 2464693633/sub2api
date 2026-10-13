//go:build windows

package creativestudio

import (
	"os"

	"golang.org/x/sys/windows"
)

func lockStorageFile(file *os.File) error {
	// All instances lock the first byte, even when the lock file is empty.
	return windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &windows.Overlapped{})
}

func unlockStorageFile(file *os.File) error {
	return windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, &windows.Overlapped{})
}
