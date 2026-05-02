# Feature Dev

`/feature-dev` starts a guided workflow for non-trivial feature development.

Usage:

```text
/feature-dev
/feature-dev add the requested feature description here
```

With no argument, Rune arms the workflow and prepends it to your next message. With an inline description, Rune starts the workflow immediately using that description.

The workflow asks Rune to:

1. Clarify the feature goal and constraints.
2. Explore the codebase with `code-explorer` subagents.
3. Ask blocking clarifying questions before architecture decisions.
4. Use `code-architect` subagents when design alternatives or focused architecture analysis help.
5. Present an implementation plan and wait for explicit approval before editing.
6. Implement surgically after approval, following project style and preserving unrelated user work.
7. Review changes with `code-reviewer` subagents and summarize validation and risks.

`/feature-dev` respects Plan Mode / Act Mode semantics. It should not use mutating tools before the user approves an implementation plan.
