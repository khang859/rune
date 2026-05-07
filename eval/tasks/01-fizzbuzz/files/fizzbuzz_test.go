package fizzbuzz

import "testing"

func TestFizzBuzz(t *testing.T) {
	cases := []struct {
		in   int
		want string
	}{
		{1, "1"},
		{2, "2"},
		{3, "Fizz"},
		{5, "Buzz"},
		{6, "Fizz"},
		{10, "Buzz"},
		{15, "FizzBuzz"},
		{30, "FizzBuzz"},
		{7, "7"},
	}
	for _, c := range cases {
		if got := FizzBuzz(c.in); got != c.want {
			t.Errorf("FizzBuzz(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}
