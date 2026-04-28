package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/khang859/rune/internal/ai"
)

type Read struct{}

func (Read) Spec() ai.ToolSpec {
	return ai.ToolSpec{
		Name:        "read",
		Description: "Read the contents of a file at an absolute path.",
		Schema: json.RawMessage(`{
            "type":"object",
            "properties":{"path":{"type":"string"}},
            "required":["path"]
        }`),
	}
}

func (Read) Run(ctx context.Context, args json.RawMessage) (Result, error) {
	var a struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return Result{Output: fmt.Sprintf("invalid args: %v", err), IsError: true}, nil
	}
	b, err := os.ReadFile(a.Path)
	if err != nil {
		return Result{Output: err.Error(), IsError: true}, nil
	}
	return Result{Output: string(b)}, nil
}
