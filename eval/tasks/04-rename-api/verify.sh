#!/usr/bin/env bash
set -e
go build ./...
go test ./...
# After rename, no bare "Parse" word should remain in the .go files.
# `grep -w` matches whole words, so it won't false-positive on "ParseInt".
if grep -rwn 'Parse' --include='*.go' .; then
  echo "found leftover bare 'Parse' references" >&2
  exit 1
fi
# And the new name must actually appear.
grep -rwn 'ParseInt' --include='*.go' . >/dev/null
