package views

import "strings"

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func trimToWidth(s string, width int) string {
	if width <= 0 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= width {
		return s
	}
	if width <= 1 {
		return string(runes[:width])
	}
	return string(runes[:width-1]) + "…"
}

func spacer(n int) string {
	if n <= 0 {
		return " "
	}
	return strings.Repeat(" ", n)
}
