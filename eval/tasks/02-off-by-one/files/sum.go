package sum

// Sum returns the total of all elements in s.
func Sum(s []int) int {
	total := 0
	for i := 0; i < len(s)-1; i++ {
		total += s[i]
	}
	return total
}
