#!/usr/bin/env bash
# Usage: eval/run.sh <provider> [model]
#   e.g. eval/run.sh codex gpt-5
#        eval/run.sh groq llama-3.3-70b
#
# Loops every directory under eval/tasks/, runs `rune --prompt` against a
# scratch workdir seeded from <task>/files/, then runs <task>/verify.sh.
# Writes one JSON line per task to eval/results/<timestamp>_<provider>_<model>.jsonl.
#
# Workdirs are left in /tmp on failure so you can inspect them. Path is printed
# in the JSONL row.

set -uo pipefail

if [[ $# -lt 1 ]]; then
  echo "usage: $0 <provider> [model]" >&2
  exit 2
fi

PROVIDER="$1"
MODEL="${2:-}"

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
EVAL_DIR="$REPO_ROOT/eval"
RUNE_BIN="${RUNE_BIN:-rune}"

if ! command -v "$RUNE_BIN" >/dev/null 2>&1 && [[ ! -x "$RUNE_BIN" ]]; then
  echo "rune binary not found (set RUNE_BIN or put 'rune' on PATH)" >&2
  exit 2
fi

ts="$(date -u +%Y%m%dT%H%M%SZ)"
results_file="$EVAL_DIR/results/${ts}_${PROVIDER}_${MODEL:-default}.jsonl"
mkdir -p "$EVAL_DIR/results"

passed=0
failed=0

shopt -s nullglob
task_dirs=("$EVAL_DIR"/tasks/*/)
shopt -u nullglob

if [[ ${#task_dirs[@]} -eq 0 ]]; then
  echo "no tasks found in $EVAL_DIR/tasks/" >&2
  exit 2
fi

for task_dir in "${task_dirs[@]}"; do
  task="$(basename "$task_dir")"
  workdir="$(mktemp -d -t "rune-eval-${task}-XXXXXX")"

  if [[ -d "$task_dir/files" ]]; then
    cp -r "$task_dir/files/." "$workdir/"
  fi

  prompt="$(cat "$task_dir/task.md")"
  transcript="$workdir/transcript.txt"

  start=$(date +%s)
  if [[ -n "$MODEL" ]]; then
    ( cd "$workdir" && "$RUNE_BIN" --provider "$PROVIDER" --model "$MODEL" --prompt "$prompt" ) >"$transcript" 2>&1
  else
    ( cd "$workdir" && "$RUNE_BIN" --provider "$PROVIDER" --prompt "$prompt" ) >"$transcript" 2>&1
  fi
  rune_exit=$?
  end=$(date +%s)

  ( cd "$workdir" && bash "$task_dir/verify.sh" ) >"$workdir/verify.log" 2>&1
  verify_exit=$?

  if [[ $verify_exit -eq 0 ]]; then
    status=pass
    passed=$((passed+1))
  else
    status=fail
    failed=$((failed+1))
  fi

  printf '%-4s %-30s %3ds  %s\n' "$status" "$task" "$((end-start))" "$workdir"

  printf '{"task":"%s","status":"%s","seconds":%d,"rune_exit":%d,"verify_exit":%d,"workdir":"%s","provider":"%s","model":"%s"}\n' \
    "$task" "$status" "$((end-start))" "$rune_exit" "$verify_exit" "$workdir" "$PROVIDER" "${MODEL:-default}" \
    >>"$results_file"
done

total=$((passed+failed))
printf '\n%d/%d passed  →  %s\n' "$passed" "$total" "$results_file"
[[ $failed -eq 0 ]]
