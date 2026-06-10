---
name: commit
description: Create well-formatted git commits. Use when committing changes, creating commits, or preparing code for commit.
disable-model-invocation: true
allowed-tools: Bash, Read
---

## CRITICAL: No hard line wrapping

Every bullet must be a single line — no exceptions. Never break a bullet across two lines.

```
WRONG:
- Ensure feature work is committed before the
  merge/next-item decision.

CORRECT:
- Ensure feature work is committed before the merge/next-item decision
```

## CRITICAL: No blank lines inside categories

Write each section header directly followed by its bullets. Keep bullets in the same section contiguous. Use exactly one blank line between categories, and do not leave a blank line after the final bullet.

```
WRONG:
Features:

- Add authentication

- Add token refresh

CORRECT:
Features:
- Add authentication
- Add token refresh

Tests:
- Cover authentication flows
```

## CRITICAL: No scope in type prefix

`feat: ...` not `feat(scope): ...`

## Commit types

`feat` / `fix` / `refactor` / `docs` / `test` / `chore` / `perf` / `style` / `ci` / `mixed`

Prefer the main category when lower-importance categories are supplementary to the primary change:
- `feat:` when a feature includes its docs, tests, examples, config, or follow-up cleanup
- `fix:` when a bug fix includes regression tests, docs, or related cleanup
- `refactor:` when refactoring includes tests or docs that explain or protect the refactor

Use `mixed:` only when the commit contains co-equal or unrelated primary categories, such as an independent feature and bug fix, a docs rewrite bundled with unrelated code changes, or multiple unrelated maintenance tasks. Do not use `mixed:` just because implementation, docs, and tests changed together for one coherent feature/fix/refactor.

## Format

**Title only** — when the title fully describes the change with no meaningful detail to add:
```
fix: correct typo in README
chore: bump lodash to 4.17.21
```

**Title + sections** — when there are multiple changes or the title alone would be ambiguous:
```
<type>: <short description (50-72 chars, imperative mood)>

Features:
- <single line>

Fixes:
- <single line>

Refactors:
- <single line>

Docs:
- <single line>

Tests:
- <single line>

Other:
- <single line>
```

Only include sections with changes. Order: Features, Fixes, Refactors, Docs, Tests, Other.

## Process

1. **Review state** — `git status`, `git diff --cached`, `git log --oneline -5`
2. **Categorise every changed file** — feat/fix/refactor/docs/test/chore — then pick the main type, treating docs/tests as supplementary when they serve the primary change
3. **Stage files** — never stage `.env` or credentials
4. **Draft message** — scan every bullet: if any continues on the next line, rewrite it as one line
5. **Commit** — use `git commit -F -` with a heredoc (or `-m`) to preserve formatting; never `--no-verify` or `--amend` unless explicitly asked

## Other rules

- No AI assistant attribution or emojis
- Never commit secrets (`.env`, credentials, keys)
