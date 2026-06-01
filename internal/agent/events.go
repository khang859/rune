package agent

import (
	"github.com/khang859/rune/internal/ai"
	"github.com/khang859/rune/internal/tools"
)

type Event interface{ isEvent() }

type AssistantText struct{ Delta string }
type ThinkingText struct{ Delta string }
type ToolStarted struct{ Call ai.ToolCall }
type ToolFinished struct {
	Call   ai.ToolCall
	Result tools.Result
}
type TurnUsage struct {
	Usage ai.Usage
	Cost  float64
}
type ContextOverflow struct{}
type TurnAborted struct{}
type TurnDone struct{ Reason string }
type TurnError struct{ Err error }

// InvalidToolCallRecovered fires when the model emitted one or more tool
// calls whose names are not in the request's tool list. The bad calls are
// dropped from session history and a nudge is appended; the agent continues
// the turn instead of failing.
type InvalidToolCallRecovered struct{ Names []string }

// RequiredToolPending fires in headless (--require-tool) mode when the model
// ended its turn without calling a required completion tool. A nudge is
// appended and the turn continues; Attempt is the 1-based nudge count.
type RequiredToolPending struct {
	Names   []string
	Attempt int
}

func (AssistantText) isEvent()            {}
func (ThinkingText) isEvent()             {}
func (ToolStarted) isEvent()              {}
func (ToolFinished) isEvent()             {}
func (TurnUsage) isEvent()                {}
func (ContextOverflow) isEvent()          {}
func (TurnAborted) isEvent()              {}
func (TurnDone) isEvent()                 {}
func (TurnError) isEvent()                {}
func (InvalidToolCallRecovered) isEvent() {}
func (RequiredToolPending) isEvent()      {}
