package util

import (
	"flag"
	"strings"
)

// FormatFlagUsage returns a string containing the usage information for the given FlagSet.
func FormatFlagUsage(fs *flag.FlagSet) string {
	var b strings.Builder
	o := fs.Output()
	fs.SetOutput(&b)
	fs.PrintDefaults()
	fs.SetOutput(o)
	return b.String()
}
