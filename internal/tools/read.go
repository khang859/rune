package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/khang859/rune/internal/ai"
)

const defaultReadLimit = 200

type Read struct{}

func (Read) Spec() ai.ToolSpec {
	return ai.ToolSpec{
		Name:        "read",
		Description: "Read lines from a file at an absolute path. Returns the first 200 lines by default. Page through larger files with `offset` (1-indexed start line) and `limit`; the truncation footer tells you the next offset to use. If you genuinely need the whole file in one shot, set `read_all`: true.",
		Schema: json.RawMessage(`{
            "type":"object",
            "properties":{
                "path":{"type":"string"},
                "offset":{"type":"integer","description":"1-indexed line to start from. Defaults to 1."},
                "limit":{"type":"integer","description":"Maximum number of lines to return. Defaults to 200."},
                "read_all":{"type":"boolean","description":"Read the entire file, ignoring offset and limit."}
            },
            "required":["path"]
        }`),
	}
}

func (Read) Run(ctx context.Context, args json.RawMessage) (Result, error) {
	var a struct {
		Path    string `json:"path"`
		Offset  int    `json:"offset"`
		Limit   int    `json:"limit"`
		ReadAll bool   `json:"read_all"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return Result{Output: fmt.Sprintf(`invalid args: %v. Expected JSON: {"path": string, "offset"?: int, "limit"?: int, "read_all"?: bool}.`, err), IsError: true}, nil
	}
	b, err := os.ReadFile(a.Path)
	if err != nil {
		return Result{Output: fmt.Sprintf("couldn't read %s: %v. Verify the path is absolute and exists; try `bash` with `ls` to inspect the directory.", a.Path, err), IsError: true}, nil
	}

	if a.ReadAll {
		return Result{Output: string(b)}, nil
	}

	if a.Offset < 0 || a.Limit < 0 {
		return Result{Output: "offset and limit must be non-negative", IsError: true}, nil
	}
	offset := a.Offset
	if offset == 0 {
		offset = 1
	}
	limit := a.Limit
	if limit == 0 {
		limit = defaultReadLimit
	}

	content := string(b)
	lines := strings.Split(content, "\n")
	hadTrailingNewline := strings.HasSuffix(content, "\n")
	if hadTrailingNewline {
		lines = lines[:len(lines)-1]
	}
	total := len(lines)

	if offset > total {
		return Result{Output: fmt.Sprintf("offset %d is past the end of %s (file has %d lines). Use a smaller offset, or omit it to start at line 1.", offset, a.Path, total), IsError: true}, nil
	}

	end := offset - 1 + limit
	if end > total {
		end = total
	}
	out := strings.Join(lines[offset-1:end], "\n")
	truncated := end < total
	if hadTrailingNewline && !truncated {
		out += "\n"
	}
	if truncated {
		out += fmt.Sprintf("\n\n[showing lines %d-%d of %d. To keep reading, call `read` again with offset=%d. To read the whole file at once, set read_all=true.]", offset, end, total, end+1)
	}
	return Result{Output: out}, nil
}
