//go:build linux

package filesystem

import (
	"os"
	"syscall"
	"time"
)

func fileAccessTimes(fi os.FileInfo) (time.Time, time.Time) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return time.Time{}, time.Time{}
	}
	// Linux ctime is *change* time, not creation time.
	return time.Time{}, time.Unix(st.Atim.Sec, st.Atim.Nsec)
}

func localZoneLabel(t time.Time) string {
	name, _ := t.Zone()
	return name
}
