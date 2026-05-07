package renameapi

import "strconv"

// Parse converts a decimal string to an int.
func Parse(s string) (int, error) {
	return strconv.Atoi(s)
}
