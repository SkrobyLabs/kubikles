---
name: commit
description: Create well-formatted git commits. Use when committing changes, creating commits, or preparing code for commit.
disable-model-invocation: true
allowed-tools: Bash, Read
---

## Task
Create a clean, well-formatted git commit following best practices.

## Process

1. **Review state** - Run `git status`, `git diff --cached`, `git log --oneline -5`

2. **Analyze changes** - Identify type (feat/fix/chore/refactor/docs/test/perf/style/ci/mixed), understand purpose, check for sensitive files

3. **Stage files** - Add relevant files (warn about sensitive files, never stage .env/credentials)

4. **Execute commit** - Draft message and commit immediately using heredoc format. User can accept/deny via permission prompt. Never use `--no-verify` or `--amend` unless requested

## Commit Types
- `feat`: New feature
- `fix`: Bug fix
- `chore`: Maintenance
- `refactor`: Code restructuring
- `docs`: Documentation
- `test`: Tests
- `perf`: Performance
- `style`: Formatting
- `ci`: CI/CD
- `mixed`: Multiple change types (use sectioned format)

## Standard Format
```
<type>: <short description>

- Bullet for each distinct change
- Focus on WHAT changed, not HOW
```

## Mixed Format (when changes span multiple types)
```
mixed: <short description>

Features:
- New capability A
- New capability B

Fixes:
- Resolved issue X

Refactors:
- Restructured Y

Docs:
- Updated Z
```

Only include sections that have changes. Order: Features, Fixes, Refactors, Docs, Tests, Other.

## Rules
- No Claude attribution or emojis in commits
- Never commit secrets (.env, credentials)
- Atomic commits when possible, mixed when necessary
- First line: 50-72 chars, imperative mood

## Examples

**Single type:**
```
feat: Add JWT-based user authentication

- Add email/password sign-in with JWT token generation
- Validate tokens via middleware on protected endpoints
- Add authentication test coverage
```

**Mixed types:**
```
mixed: Update authentication system

Features:
- Add password reset flow
- Add remember-me option

Fixes:
- Resolve session timeout on mobile

Docs:
- Update auth API documentation
```
