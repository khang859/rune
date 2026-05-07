#!/usr/bin/env bash
set -e
test -f user/user.go
grep -q '^package user' user/user.go
grep -q 'type User' user/user.go
if grep -q 'type User' main.go; then
  echo "type User must no longer be defined in main.go" >&2
  exit 1
fi
go build ./...
go test ./...
