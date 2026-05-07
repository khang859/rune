`go test ./...` hangs because of a deadlock in `pipeline.go`. Fix it without changing the public signature of `Sum`.
