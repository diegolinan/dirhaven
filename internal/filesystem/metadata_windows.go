//go:build windows

package filesystem

import (
	"os"
	"syscall"
	"time"
)

// fileAccessTimes exposes native Windows file metadata without relying on
// PowerShell or commands. Values are taken from the same stat operation.
func fileAccessTimes(fi os.FileInfo) (time.Time, time.Time) {
	a, ok := fi.Sys().(*syscall.Win32FileAttributeData)
	if !ok {
		return time.Time{}, time.Time{}
	}
	created := time.Unix(0, a.CreationTime.Nanoseconds())
	accessed := time.Unix(0, a.LastAccessTime.Nanoseconds())
	return created, accessed
}

func localZoneLabel(t time.Time) string {
	var info syscall.Timezoneinformation
	rc, err := syscall.GetTimeZoneInformation(&info)
	if err != nil {
		name, _ := t.Zone()
		return name
	}
	var label string
	if rc == 2 {
		label = syscall.UTF16ToString(info.DaylightName[:])
	}
	if label == "" {
		label = syscall.UTF16ToString(info.StandardName[:])
	}
	if label == "" {
		label, _ = t.Zone()
	}
	return label
}
