//go:build !windows

package filesystem

import "errors"

func dynamicDriveDiscoverySupported() bool { return false }

func discoverDriveRoots() ([]string, error) {
	return nil, errors.New("Windows drive-letter wildcard is not supported on this OS")
}
