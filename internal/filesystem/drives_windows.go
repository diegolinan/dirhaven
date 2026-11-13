//go:build windows

package filesystem

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// GetLogicalDrives is queried on every filesystem operation. A newly mapped
// drive letter, removable disk or newly attached volume needs no MCP restart.
// Only usable drive-letter roots are published; arbitrary UNC/device paths are
// not wildcard-expanded.
var getLogicalDrives = syscall.NewLazyDLL("kernel32.dll").NewProc("GetLogicalDrives")

func dynamicDriveDiscoverySupported() bool { return true }

func discoverDriveRoots() ([]string, error) {
	mask, _, callErr := getLogicalDrives.Call()
	if mask == 0 {
		if callErr != nil && !errors.Is(callErr, syscall.Errno(0)) {
			return nil, fmt.Errorf("GetLogicalDrives: %w", callErr)
		}
		return nil, errors.New("GetLogicalDrives returned no drives")
	}

	roots := make([]string, 0, 8)
	for i := uint(0); i < 26; i++ {
		if mask&(1<<i) == 0 {
			continue
		}
		drive := string(rune('A'+i)) + ":\\"
		fi, err := os.Stat(drive)
		if err != nil || !fi.IsDir() {
			// Empty optical drives, inaccessible/disconnected network
			// mounts and transiently removed volumes must not be offered.
			continue
		}
		// Keep the drive letter as the authorized root. EvalSymlinks can
		// translate a mapped drive into UNC, invalidating X:\ paths.
		roots = append(roots, filepath.Clean(drive))
	}
	return roots, nil
}
