package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/khang859/rune/internal/agent"
	"github.com/khang859/rune/internal/ai"
	"github.com/khang859/rune/internal/ai/faux"
	"github.com/khang859/rune/internal/session"
	"github.com/khang859/rune/internal/tools"
)

type scriptFile struct {
	Provider    string     `json:"provider"`
	Session     string     `json:"session"`
	Model       string     `json:"model"`
	Faux        []fauxStep `json:"faux"`
	UserMessage string     `json:"user_message"`
}

type fauxStep struct {
	Reply    string          `json:"reply,omitempty"`
	Thinking string          `json:"thinking,omitempty"`
	Tool     *fauxToolCall   `json:"tool,omitempty"`
	Done     bool            `json:"done,omitempty"`
	Overflow bool            `json:"overflow,omitempty"`
}

type fauxToolCall struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}

// runScript drives one turn from the script and writes a transcript to w.
// fauxBase is injected for testability; pass faux.New() in main.
func runScript(ctx context.Context, path string, w io.Writer, _ *faux.Faux) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var sc scriptFile
	if err := json.Unmarshal(b, &sc); err != nil {
		return err
	}

	f := faux.New()
	for _, st := range sc.Faux {
		switch {
		case st.Reply != "":
			f.Reply(st.Reply)
		case st.Thinking != "":
			f.Thinking(st.Thinking)
		case st.Tool != nil:
			f.CallTool(st.Tool.Name, string(st.Tool.Args))
		case st.Overflow:
			f.DoneOverflow()
		case st.Done:
			f.Done()
		}
	}

	sess := session.New(sc.Model)
	if sc.Session != "" {
		sess.SetPath(sc.Session)
	}
	reg := tools.NewRegistry()
	reg.Register(tools.Read{})
	reg.Register(tools.Write{})
	reg.Register(tools.Edit{})
	reg.Register(tools.Bash{})

	a := agent.New(f, reg, sess, "")
	msg := ai.Message{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextBlock{Text: sc.UserMessage}}}
	for ev := range a.Run(ctx, msg) {
		switch v := ev.(type) {
		case agent.AssistantText:
			fmt.Fprint(w, v.Delta)
		case agent.ToolStarted:
			fmt.Fprintf(w, "\n[tool start: %s]", v.Call.Name)
		case agent.ToolFinished:
			fmt.Fprintf(w, "\n[tool done: %s -> %q]", v.Call.Name, truncate(v.Result.Output, 80))
		case agent.TurnError:
			fmt.Fprintf(w, "\n[error: %v]", v.Err)
		case agent.TurnDone:
			fmt.Fprintf(w, "\n[done: %s]", v.Reason)
		}
	}
	if sc.Session != "" {
		if err := sess.Save(); err != nil {
			return fmt.Errorf("save session: %w", err)
		}
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
