package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/khang859/rune/internal/ai"
)

type SubagentManager interface {
	Spawn(ctx context.Context, req SpawnSubagentRequest) (*SubagentTask, error)
	List() []SubagentTask
	Get(id string) *SubagentTask
	Cancel(id string) error
}

const subagentsDisabledError = "subagents are disabled in settings"

type disabledSubagentManager struct{}

type disabledSubagentMarker interface {
	subagentsDisabled() bool
}

func DisabledSubagentManager() SubagentManager { return disabledSubagentManager{} }

func (disabledSubagentManager) subagentsDisabled() bool { return true }

func subagentManagerDisabled(m SubagentManager) bool {
	d, ok := m.(disabledSubagentMarker)
	return ok && d.subagentsDisabled()
}

func (disabledSubagentManager) Spawn(ctx context.Context, req SpawnSubagentRequest) (*SubagentTask, error) {
	_ = ctx
	_ = req
	return nil, fmt.Errorf(subagentsDisabledError)
}

func (disabledSubagentManager) List() []SubagentTask { return nil }

func (disabledSubagentManager) Get(id string) *SubagentTask {
	_ = id
	return nil
}

func (disabledSubagentManager) Cancel(id string) error {
	_ = id
	return fmt.Errorf(subagentsDisabledError)
}

type SpawnSubagentRequest struct {
	Name         string
	Prompt       string
	AgentType    string
	Background   bool
	Dependencies []string
	TimeoutSecs  int
}

type SubagentTask struct {
	ID           string     `json:"task_id"`
	Name         string     `json:"name"`
	FamiliarName string     `json:"familiar_name,omitempty"`
	AgentType    string     `json:"agent_type"`
	Status       string     `json:"status"`
	Dependencies []string   `json:"dependencies,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
	Summary      string     `json:"summary,omitempty"`
	Error        string     `json:"error,omitempty"`
}

type SpawnSubagent struct{ Manager SubagentManager }

type ListSubagents struct{ Manager SubagentManager }

type GetSubagentResult struct{ Manager SubagentManager }

type CancelSubagent struct{ Manager SubagentManager }

func RegisterSubagentTools(r *Registry, m SubagentManager) {
	if m == nil {
		return
	}
	r.Register(SpawnSubagent{Manager: m})
	r.Register(ListSubagents{Manager: m})
	r.Register(GetSubagentResult{Manager: m})
	r.Register(CancelSubagent{Manager: m})
}

func (SpawnSubagent) Spec() ai.ToolSpec {
	return ai.ToolSpec{Name: "spawn_subagent", Description: "Start a specialized subagent with isolated context. Supports read-only general, exploration, and validator subagents and can run them in the background.", Schema: json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"},"prompt":{"type":"string"},"agent_type":{"type":"string","default":"general","description":"Subagent type/name. Available: general, exploration, validator."},"background":{"type":"boolean","default":true},"dependencies":{"type":"array","items":{"type":"string"},"description":"Optional subagent task IDs that must complete before this task starts."},"timeout_secs":{"type":"integer","default":600}},"required":["name","prompt"]}`)}
}

func (t SpawnSubagent) Run(ctx context.Context, args json.RawMessage) (Result, error) {
	var in struct {
		Name         string   `json:"name"`
		Prompt       string   `json:"prompt"`
		AgentType    string   `json:"agent_type"`
		Background   *bool    `json:"background"`
		Dependencies []string `json:"dependencies"`
		TimeoutSecs  int      `json:"timeout_secs"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return Result{Output: "invalid JSON: " + err.Error(), IsError: true}, nil
	}
	bg := true
	if in.Background != nil {
		bg = *in.Background
	}
	task, err := t.Manager.Spawn(ctx, SpawnSubagentRequest{Name: in.Name, Prompt: in.Prompt, AgentType: in.AgentType, Background: bg, Dependencies: in.Dependencies, TimeoutSecs: in.TimeoutSecs})
	if err != nil {
		return Result{Output: err.Error(), IsError: true}, nil
	}
	return jsonResult(task)
}

func (ListSubagents) Spec() ai.ToolSpec {
	return ai.ToolSpec{Name: "list_subagents", Description: "List active and recent subagent tasks with statuses.", Schema: json.RawMessage(`{"type":"object","properties":{}}`)}
}

func (t ListSubagents) Run(ctx context.Context, args json.RawMessage) (Result, error) {
	_ = ctx
	if len(strings.TrimSpace(string(args))) > 0 && strings.TrimSpace(string(args)) != "{}" {
		var discard map[string]any
		if err := json.Unmarshal(args, &discard); err != nil {
			return Result{Output: "invalid JSON: " + err.Error(), IsError: true}, nil
		}
	}
	if subagentManagerDisabled(t.Manager) {
		return Result{Output: subagentsDisabledError, IsError: true}, nil
	}
	return jsonResult(map[string]any{"tasks": t.Manager.List()})
}

func (GetSubagentResult) Spec() ai.ToolSpec {
	return ai.ToolSpec{Name: "get_subagent_result", Description: "Retrieve the current status or final result for a subagent task.", Schema: json.RawMessage(`{"type":"object","properties":{"task_id":{"type":"string"}},"required":["task_id"]}`)}
}

func (t GetSubagentResult) Run(ctx context.Context, args json.RawMessage) (Result, error) {
	_ = ctx
	var in struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return Result{Output: "invalid JSON: " + err.Error(), IsError: true}, nil
	}
	if strings.TrimSpace(in.TaskID) == "" {
		return Result{Output: "task_id is required", IsError: true}, nil
	}
	if subagentManagerDisabled(t.Manager) {
		return Result{Output: subagentsDisabledError, IsError: true}, nil
	}
	task := t.Manager.Get(in.TaskID)
	if task == nil {
		return Result{Output: fmt.Sprintf("unknown subagent task %q", in.TaskID), IsError: true}, nil
	}
	return jsonResult(task)
}

func (CancelSubagent) Spec() ai.ToolSpec {
	return ai.ToolSpec{Name: "cancel_subagent", Description: "Cancel a pending, blocked, or running subagent task.", Schema: json.RawMessage(`{"type":"object","properties":{"task_id":{"type":"string"}},"required":["task_id"]}`)}
}

func (t CancelSubagent) Run(ctx context.Context, args json.RawMessage) (Result, error) {
	_ = ctx
	var in struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return Result{Output: "invalid JSON: " + err.Error(), IsError: true}, nil
	}
	if err := t.Manager.Cancel(in.TaskID); err != nil {
		return Result{Output: err.Error(), IsError: true}, nil
	}
	return jsonResult(map[string]any{"task_id": in.TaskID, "status": "cancelled"})
}

func jsonResult(v any) (Result, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return Result{}, err
	}
	return Result{Output: string(b)}, nil
}
