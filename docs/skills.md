# Skills

A skill is a markdown file. Drop one into `~/.rune/skills/` (user-global) or
`./.rune/skills/` (project-local; overrides user-global on slug collision).
The filename minus `.md` becomes the slug — `refactor-step.md` → `/skill:refactor-step`.

## Lifecycle

1. rune scans the two skill roots at startup and on `/reload`.
2. Each skill becomes a `/skill:<slug>` command in the slash menu.
3. Selecting a skill **arms** its body. The body is prepended to your next
   submitted message and then cleared.

## Authoring tips

- Be specific. "When refactoring, write a failing test first" beats "be careful".
- One skill per file; don't bury two unrelated workflows in one body.
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
