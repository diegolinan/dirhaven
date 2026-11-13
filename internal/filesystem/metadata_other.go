//go:build !windows && !linux && !darwin

package filesystem

import (
	"os"
	"time"
)

func fileAccessTimes(_ os.FileInfo) (time.Time, time.Time) { return time.Time{}, time.Time{} }

func localZoneLabel(t time.Time) string {
	name, _ := t.Zone()
	return name
}
