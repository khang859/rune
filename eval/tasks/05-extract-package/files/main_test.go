package app

import "testing"

func TestRunDemo(t *testing.T) {
	if got := RunDemo("World"); got != "Hello, World" {
		t.Errorf("RunDemo(World) = %q, want %q", got, "Hello, World")
	}
}

func TestUserGreet(t *testing.T) {
	u := User{Name: "Alice"}
	if got := u.Greet(); got != "Hello, Alice" {
		t.Errorf("Greet() = %q, want %q", got, "Hello, Alice")
	}
}
