`go test -race ./...` reports a data race in `counter.go`. Fix it while keeping the public API (`New`, `Inc`, `Value`) unchanged.
