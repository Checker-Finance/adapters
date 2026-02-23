package utils

import "regexp"

var dsnPasswordRegex = regexp.MustCompile(`(:)([^:@]+)(@)`)

func MaskDSN(dsn string) string {
	return dsnPasswordRegex.ReplaceAllString(dsn, ":***@")
}
