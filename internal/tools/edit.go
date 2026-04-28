package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/khang859/rune/internal/ai"
)

type Edit struct{}

func (Edit) Spec() ai.ToolSpec {
	return ai.ToolSpec{
		Name:        "edit",
		Description: "Replace a unique exact-string occurrence in a file. Fails if old_string is missing or appears more than once.",
		Schema: json.RawMessage(`{
            "type":"object",
            "properties":{
                "path":{"type":"string"},
                "old_string":{"type":"string"},
                "new_string":{"type":"string"}
            },
            "required":["path","old_string","new_string"]
        }`),
	}
}

func (Edit) Run(ctx context.Context, args json.RawMessage) (Result, error) {
	var a struct {
		Path      string `json:"path"`
		OldString string `json:"old_string"`
		NewString string `json:"new_string"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return Result{Output: fmt.Sprintf("invalid args: %v", err), IsError: true}, nil
	}
	b, err := os.ReadFile(a.Path)
	if err != nil {
		return Result{Output: err.Error(), IsError: true}, nil
	}
	src := string(b)
	count := strings.Count(src, a.OldString)
	if count == 0 {
		return Result{Output: fmt.Sprintf("old_string not found in %s", a.Path), IsError: true}, nil
	}
	if count > 1 {
		return Result{Output: fmt.Sprintf("old_string appears %d times in %s — must be unique", count, a.Path), IsError: true}, nil
	}
	out := strings.Replace(src, a.OldString, a.NewString, 1)
	if err := os.WriteFile(a.Path, []byte(out), 0o644); err != nil {
		return Result{Output: err.Error(), IsError: true}, nil
	}
	return Result{Output: fmt.Sprintf("edited %s", a.Path)}, nil
}
