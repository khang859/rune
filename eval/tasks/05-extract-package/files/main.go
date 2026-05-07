package app

type User struct {
	Name string
}

func (u User) Greet() string {
	return "Hello, " + u.Name
}

func RunDemo(name string) string {
	return User{Name: name}.Greet()
}
