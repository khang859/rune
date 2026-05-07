#!/usr/bin/env bash
# Print total line count across all .txt files in the given directory.
set -u

dir="${1:-.}"
total=0

find "$dir" -maxdepth 1 -type f -name '*.txt' | while read -r f; do
    n=$(wc -l < "$f")
    total=$((total + n))
done

echo "$total"
