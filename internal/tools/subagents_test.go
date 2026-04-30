package tools

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/khang859/rune/internal/ai"
)

type recordingSubagentManager struct {
	req SpawnSubagentRequest
}

func (m *recordingSubagentManager) Spawn(ctx context.Context, req SpawnSubagentRequest) (*SubagentTask, error) {
	_ = ctx
	m.req = req
	return &SubagentTask{
		ID:           "subagent_1",
		Name:         req.Name,
		AgentType:    req.AgentType,
		Status:       "blocked",
		Dependencies: append([]string(nil), req.Dependencies...),
		CreatedAt:    time.Now(),
	}, nil
}

func (m *recordingSubagentManager) List() []SubagentTask { return nil }
func (m *recordingSubagentManager) Get(id string) *SubagentTask {
	_ = id
	return nil
}
func (m *recordingSubagentManager) Cancel(id string) error {
	_ = id
	return nil
}

func TestSpawnSubagentRunParsesDependencies(t *testing.T) {
	mgr := &recordingSubagentManager{}
	reg := NewRegistry()
	RegisterSubagentTools(reg, mgr)

	res, err := reg.Run(context.Background(), ai.ToolCall{
		Name: "spawn_subagent",
		Args: json.RawMessage(`{"name":"child","prompt":"do it","dependencies":["subagent_1","subagent_2"],"background":true}`),
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if res.IsError {
		t.Fatalf("result is error: %s", res.Output)
	}
	want := []string{"subagent_1", "subagent_2"}
	if !reflect.DeepEqual(mgr.req.Dependencies, want) {
		t.Fatalf("dependencies = %#v, want %#v", mgr.req.Dependencies, want)
	}
	if !strings.Contains(res.Output, `"dependencies": [`) || !strings.Contains(res.Output, `"subagent_1"`) {
		t.Fatalf("output missing dependencies: %s", res.Output)
	}
}

func TestSpawnSubagentSpecAdvertisesDependencies(t *testing.T) {
	spec := (SpawnSubagent{}).Spec()
	var schema struct {
		Properties map[string]struct {
			Type        string `json:"type"`
			Description string `json:"description"`
			Items       *struct {
				Type string `json:"type"`
			} `json:"items"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(spec.Schema, &schema); err != nil {
		t.Fatalf("schema unmarshal: %v", err)
	}
	deps, ok := schema.Properties["dependencies"]
	if !ok {
		t.Fatalf("dependencies property missing from schema: %s", string(spec.Schema))
	}
	if deps.Type != "array" || deps.Items == nil || deps.Items.Type != "string" {
		t.Fatalf("dependencies schema = %+v, want array of strings", deps)
	}
	if !strings.Contains(deps.Description, "must complete") {
		t.Fatalf("dependencies description = %q, want completion semantics", deps.Description)
	}
}

func TestCancelSubagentSpecMentionsBlocked(t *testing.T) {
	spec := (CancelSubagent{}).Spec()
	if !strings.Contains(strings.ToLower(spec.Description), "blocked") {
		t.Fatalf("description = %q, want mention of blocked tasks", spec.Description)
	}
}
