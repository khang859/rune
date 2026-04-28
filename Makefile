.PHONY: build test vet fmt lint all release-snapshot release

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.Version=$(VERSION)

build:
	go build -ldflags "$(LDFLAGS)" -o rune ./cmd/rune

test:
	go test -race ./...

vet:
	go vet ./...

fmt:
	gofmt -l .

lint:
	staticcheck ./...

all: vet fmt test build

# Cross-compile binaries into ./dist for the four common targets.
release-snapshot:
	@mkdir -p dist
	GOOS=darwin  GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o dist/rune-darwin-arm64 ./cmd/rune
	GOOS=darwin  GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o dist/rune-darwin-amd64 ./cmd/rune
	GOOS=linux   GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o dist/rune-linux-amd64 ./cmd/rune
	GOOS=linux   GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o dist/rune-linux-arm64 ./cmd/rune

release: release-snapshot
	@echo "Tagging and uploading release $(VERSION)"
	@test -n "$(VERSION)" || (echo "VERSION required"; exit 1)
	gh release create $(VERSION) dist/* --generate-notes
