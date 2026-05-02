You are helping publish a new Rune release.

Follow this exact release workflow unless the user specifies a different version or release channel.

## Goal

Create and push the next version tag, let GitHub Actions publish the release, verify the workflow succeeds, and report the release URL.

## Preconditions

1. Check repository state:
   - Run `git status`.
   - Confirm the working tree is clean.
   - Confirm the current branch is `main` and is up to date with `origin/main`.

2. If there are uncommitted changes:
   - Do not include them silently.
   - Ask whether to commit them first.
   - If the user already asked to commit/push, commit first, push to `main`, then continue.

3. Do not publish a release from an unpushed commit.

## Determine the next version

1. Inspect existing releases/tags:
   - `gh release list --limit 10`
   - `git tag --sort=-version:refname | head -20`

2. Default version bump:
   - Increment the patch version.
   - Example: latest `v0.1.12` → next `v0.1.13`.

3. If the user specifies a version, use that exact version.
   - Version tags should use the `vX.Y.Z` format unless the repo already uses something else.

4. If the target tag already exists, stop and ask what to do.

## Validate before tagging

Run an appropriate validation command before creating the tag.

For this Rune repo, default to:

```sh
go test ./internal/tui
```

If the release includes broader backend, agent, provider, or tool changes, use:

```sh
go test ./...
```

If tests fail:
- Stop.
- Summarize the failure.
- Do not tag or publish.

## Publish

This repo publishes releases from pushed `v*` tags using `.github/workflows/release.yml`.

Use the tag workflow, not a local `make release`, unless the user explicitly asks for a manual release.

Commands:

```sh
git tag <version>
git push origin <version>
```

Example:

```sh
git tag v0.1.13
git push origin v0.1.13
```

## Verify GitHub Actions

After pushing the tag, find the release workflow run:

```sh
gh run list --workflow release --limit 3 --json databaseId,status,conclusion,headBranch,displayTitle,createdAt,url
```

Identify the run whose `headBranch` is the release tag.

Poll until it completes:

```sh
gh run view <run-id> --json status,conclusion,url
```

If the workflow is still running, wait and poll again.

If the workflow fails:
- Report the run URL.
- Do not claim the release was published.
- Inspect the failed run if useful.

## Verify the GitHub release

After the workflow succeeds, verify the release exists:

```sh
gh release view <version> --json name,tagName,url,publishedAt,isDraft,isPrerelease
```

Confirm:
- `isDraft` is false.
- The tag matches the requested version.
- A release URL is present.

## Final response format

Keep the final response concise:

```text
Published new version <version>.

Details:
- Created and pushed tag: <version>
- Release workflow completed successfully.
- GitHub release: <url>
```

If anything failed, use:

```text
Release <version> was not published.

What happened:
- <brief failure summary>

Next step:
- <recommended fix>
```
