package braza

import (
	"strconv"
	"strings"
	"time"
)

func parseInt(s string) int64 {
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0
	}
	return n
}

func parseIntString(i int) string {
	str := strconv.FormatInt(int64(i), 10)
	return str
}

func isPastCutoff(cutoffTime time.Time) bool {
	now := time.Now().UTC()

	// Create today's cutoff by combining today's date with the cutoff time
	todaysCutoff := time.Date(
		now.Year(), now.Month(), now.Day(),
		cutoffTime.Hour(), cutoffTime.Minute(), cutoffTime.Second(), 0,
		time.UTC,
	)

	return now.After(todaysCutoff)
}
