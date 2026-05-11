package util

// FirstNonEmpty returns the first non-empty string from the given values.
func FirstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// FirstNonZero returns the first non-zero int from the given values.
func FirstNonZero(vals ...int) int {
	for _, v := range vals {
		if v != 0 {
			return v
		}
	}
	return 0
}
