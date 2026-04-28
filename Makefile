.PHONY: test vet fmt build all

build:
	go build -o rune ./cmd/rune

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -l .

all: vet fmt test build
