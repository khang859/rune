package pipeline

// Sum streams nums through an unbuffered channel and returns their total.
func Sum(nums []int) int {
	ch := make(chan int)

	go func() {
		for _, n := range nums {
			ch <- n
		}
	}()

	total := 0
	for v := range ch {
		total += v
	}
	return total
}
