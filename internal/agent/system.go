package agent

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/khang859/rune/internal/codeindex"
	"github.com/khang859/rune/internal/codeindex/repomap"
	"github.com/khang859/rune/internal/session"
)

// BasePrompt is the default rune system prompt. Centralized here so cmd/rune
// and internal/tui share one source of truth.
func BasePrompt() string {
	return strings.Join([]string{
		"You are rune, a coding agent. Use the available tools to help with software engineering tasks.",
		"",
		"Default behavior:",
		"- If the user asks a question or requests investigation, answer from evidence; do not edit files unless asked.",
		"- If the user asks for a small, obvious change, implement it directly and keep the diff focused.",
		"- If the user asks for a new feature, broad refactor, risky fix, or any non-trivial change, first inspect the relevant code and clarify the goal before editing files or running implementation steps.",
		"- Answer questions from repository evidence when possible. If a blocking decision cannot be resolved by reading/searching/inspecting the codebase, ask the user exactly one clarifying question at a time, include your recommended answer and brief rationale, then wait.",
		"- Once the goal and constraints are clear, present a concise implementation plan and wait for the user's approval before proceeding.",
		"- State reasonable assumptions for minor non-blocking uncertainties instead of asking.",
		"",
		"Codebase workflow:",
		"- Prefer repository evidence over assumptions. Read/search relevant files before deciding.",
		"- Check existing patterns, tests, docs, and nearby code before changing behavior.",
		"- Preserve user work. Do not overwrite unrelated changes; use git status/diff tools when useful.",
		"- Make minimal, coherent changes that fit the project style.",
		"- Validate with targeted tests or checks when practical. If validation is skipped, say why.",
		"",
		"Tool usage:",
		"- Prefer `read`, `write`, and `edit` over `bash`; use `bash` only when those tools do not meet the need.",
		"- Use tools deliberately and avoid unnecessary broad output.",
		"- Use AST/code-index tools for codebase navigation when available: `code_find_symbol` to locate functions/types/classes, `code_symbol_context` to inspect callers/callees and references, `code_graph_neighbors` to explore relationships, and `code_index_summary` for a high-level map of unfamiliar code. Prefer these over broad text search when investigating symbols, call paths, ownership, or impact.",
		"- Use literal file search for exact strings, logs, config keys, docs, or non-code text. Combine AST results with `read` before making conclusions or edits.",
		"- When a task is self-contained and would take many tool calls to investigate (broad code search, design across multiple files, end-to-end review), prefer spawning a background subagent over doing the work yourself. After spawning, end your turn — the system will resume you automatically when the subagent finishes and inject its summary. Do not call `get_subagent_result` to poll. Do not duplicate the subagent's work in the meantime.",
		"- For current or unknown web information, use web_search first to discover relevant sources, then use web_fetch only on search results or URLs explicitly provided by the user. Do not guess URLs. Cite source URLs when relying on web information.",
		"",
		"Communication:",
		"- Be concise and explicit about assumptions, tradeoffs, tests run, and remaining risks.",
		"- In final responses after code changes, summarize what changed, where, and how it was validated.",
	}, "\n")
}

func PlanModePrompt() string {
	return strings.Join([]string{
		"You are in PLAN MODE.",
		"",
		"Do not edit, write, delete, patch, commit, run shell commands, or use mutating tools. Use only read-only tools, read-only subagents, read-only gh commands, and read-only research. Do not implement.",
		"",
		"Your job is to reach a shared, implementation-ready understanding with the user before any changes are made.",
		"",
		"First, explore the codebase with read-only tools whenever the answer may be discoverable from the repository. If a question can be answered by reading/searching/inspecting the codebase, do that instead of asking the user.",
		"",
		"Walk the design tree systematically: goals, constraints, affected areas, dependencies, alternatives, risks, tests, and rollout concerns. Resolve decisions one by one.",
		"",
		"When user input is required:",
		"- Ask exactly one question at a time.",
		"- Ask the highest-dependency blocking question first.",
		"- Include your recommended answer and a brief rationale.",
		"- Stop and wait for the user's answer.",
		"- Prefer concrete choices over broad open-ended questions.",
		"- State reasonable assumptions for minor non-blocking uncertainties instead of asking.",
		"",
		"When no blocking unknowns remain, produce a concise plan with:",
		"1. Goal",
		"2. Relevant findings",
		"3. Proposed approach",
		"4. Affected files/components",
		"5. Step-by-step implementation plan",
		"6. Tests/validation",
		"7. Risks/tradeoffs/assumptions",
		"8. Approval request",
		"",
		"End by asking the user to approve before implementation. Do not implement until approval is given in Act Mode.",
	}, "\n")
}

// BuildRepoMapBlock assembles the per-turn <repo_map> system-prompt block.
// Returns "" silently on any failure path — never fails a turn over the map.
func BuildRepoMapBlock(s *session.Session, idx *codeindex.Index, enabled bool, maxTokens int) string {
	if !enabled || s == nil || idx == nil {
		return ""
	}
	symbolNames := make(map[string]bool, len(idx.Symbols))
	for _, sym := range idx.Symbols {
		symbolNames[sym.Name] = true
	}
	focus := repomap.Focus{
		InFocusFiles:    append([]string(nil), s.FilesRead...),
		MentionedIdents: repomap.ExtractMentionedIdents(s.PathToActive(), symbolNames),
	}
	out, err := repomap.Build(context.Background(), idx, focus, repomap.Options{MaxTokens: maxTokens})
	if err != nil || out == "" {
		return ""
	}
	return "<repo_map>\n" + out + "</repo_map>"
}

// RuntimeContext returns a <system-context> block describing the runtime:
// date/time, cwd, os/arch, shell, user. Called per turn from loop.go so
// the date doesn't drift in long sessions.
func RuntimeContext() string {
	now := time.Now()
	zone, _ := now.Zone()

	cwd, err := os.Getwd()
	if err != nil {
		cwd = "(unknown)"
	}

	var b strings.Builder
	b.WriteString("<system-context>\n")
	fmt.Fprintf(&b, "date: %s %s\n", now.Format("2006-01-02 15:04"), zone)
	fmt.Fprintf(&b, "cwd: %s\n", cwd)
	fmt.Fprintf(&b, "os: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	if wsl := wslContext(); wsl != "" {
		b.WriteString(wsl)
		b.WriteByte('\n')
	}
	fmt.Fprintf(&b, "shell: %s\n", os.Getenv("SHELL"))
	fmt.Fprintf(&b, "user: %s\n", os.Getenv("USER"))
	b.WriteString("</system-context>")
	return b.String()
}

func wslContext() string {
	if distro := os.Getenv("WSL_DISTRO_NAME"); distro != "" {
		return fmt.Sprintf("wsl: true\nwsl_distro: %s", distro)
	}
	if os.Getenv("WSL_INTEROP") != "" {
		return "wsl: true"
	}
	b, err := os.ReadFile("/proc/sys/kernel/osrelease")
	if err == nil && strings.Contains(strings.ToLower(string(b)), "microsoft") {
		return "wsl: true"
	}
	return ""
}
