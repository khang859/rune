# Skills

A skill is a markdown file. Drop one into `~/.rune/skills/` (user-global) or
`./.rune/skills/` (project-local; overrides user-global on slug collision).
The filename minus `.md` becomes the slug — `refactor-step.md` → `/skill:refactor-step`.

## Lifecycle

1. rune scans the two skill roots at startup and on `/reload`.
2. Each skill becomes a `/skill:<slug>` command in the slash menu.
3. Selecting a skill **arms** its body. The body is prepended to your next
   submitted message and then cleared.

## Creating skills

Use `/skill-creator` for guided help drafting or improving a skill. It arms a
built-in prompt; send your next message describing the workflow you want to
capture, or paste an existing skill you want to refine.

Good source material includes real tasks, corrections you gave rune, project
conventions, examples, edge cases, runbooks, and desired input/output formats.
After saving a generated skill, run `/reload` if rune is already open.

## Authoring tips

- Be specific. "When refactoring, write a failing test first" beats "be careful".
- One skill per file; don't bury two unrelated workflows in one body.
- Focus on what rune would otherwise get wrong: project conventions, domain
  gotchas, fragile sequences, required commands, or expected output formats.
- Prefer concrete procedures, defaults, examples, and validation steps over
  generic advice.
- Keep skills concise. Since rune loads the whole file, every line competes with
  the user's request and the rest of the conversation.
- The body is just text — there's no schema, no front matter.

## Example

`~/.rune/skills/tdd.md`:

```
Before changing implementation code, write a failing test that captures
the desired behavior. Run the test to confirm it fails. Then implement
the minimal change to make the test pass.
```

After `/skill:tdd` and "add a foo() helper", rune sends both as the user
message.
