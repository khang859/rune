#!/usr/bin/env bash
set -e
got=$(bash count_lines.sh .)
want=8
if [[ "$got" != "$want" ]]; then
    echo "count_lines.sh . = $got, want $want" >&2
    exit 1
fi
echo "ok"
