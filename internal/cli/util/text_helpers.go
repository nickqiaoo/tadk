package util

import "strings"

func CenterString(s string, w int) string {
	sw := w - len(s)
	lw := sw / 2
	rw := sw - lw
	return strings.Repeat(" ", lw) + s + strings.Repeat(" ", rw)
}
