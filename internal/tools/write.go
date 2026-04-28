package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/khang859/rune/internal/ai"
)

type Write struct{}

func (Write) Spec() ai.ToolSpec {
	return ai.ToolSpec{
		Name:        "write",
		Description: "Write content to a file. Creates parent directories. Overwrites existing files.",
		Schema: json.RawMessage(`{
            "type":"object",
            "properties":{
                "path":{"type":"string"},
                "content":{"type":"string"}
            },
            "required":["path","content"]
        }`),
	}
}

func (Write) Run(ctx context.Context, args json.RawMessage) (Result, error) {
	var a struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return Result{Output: fmt.Sprintf(`invalid args: %v. Expected JSON: {"path": string, "content": string}.`, err), IsError: true}, nil
	}
	if err := os.MkdirAll(filepath.Dir(a.Path), 0o755); err != nil {
		return Result{Output: fmt.Sprintf("couldn't create parent directory for %s: %v. Check the path and write permissions.", a.Path, err), IsError: true}, nil
	}
	if err := os.WriteFile(a.Path, []byte(a.Content), 0o644); err != nil {
		return Result{Output: fmt.Sprintf("couldn't write %s: %v. Check the path and write permissions.", a.Path, err), IsError: true}, nil
	}
	return Result{Output: fmt.Sprintf("wrote %d bytes to %s", len(a.Content), a.Path)}, nil
}
