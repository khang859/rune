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

func (AssistantText) isEvent()   {}
func (ThinkingText) isEvent()    {}
func (ToolStarted) isEvent()     {}
func (ToolFinished) isEvent()    {}
func (TurnUsage) isEvent()       {}
func (ContextOverflow) isEvent() {}
func (TurnAborted) isEvent()     {}
func (TurnDone) isEvent()        {}
func (TurnError) isEvent()       {}
