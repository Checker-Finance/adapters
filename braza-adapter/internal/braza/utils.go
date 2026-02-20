package braza

import "strconv"

func parseIntString(i int) string {
	return strconv.FormatInt(int64(i), 10)
}
