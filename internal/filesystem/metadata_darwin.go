//go:build darwin

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
	return time.Unix(st.Birthtimespec.Sec, st.Birthtimespec.Nsec), time.Unix(st.Atimespec.Sec, st.Atimespec.Nsec)
}

func localZoneLabel(t time.Time) string {
	name, _ := t.Zone()
	return name
}
