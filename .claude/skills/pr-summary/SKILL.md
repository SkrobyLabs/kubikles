---
name: pr-summary
description: Generate PR summary from branch commits. Use when reviewing commits, preparing PR, or before pushing.
allowed-tools: Bash
---

## Task
Generate a PR title and summary showing the **net effect** of all commits, not the journey.

## Process

1. **Detect branch and base**
   ```bash
   git rev-parse --abbrev-ref HEAD
   git remote show origin | grep "HEAD branch" | cut -d: -f2 | xargs
   ```

2. **Check if on main/master** - warn and stop if so

3. **Get commits and diff**
   ```bash
   git log --format="%s" $(git merge-base HEAD main)..HEAD
   git diff --stat $(git merge-base HEAD main)..HEAD
   ```

4. **Analyze for net changes** (CRITICAL)

   **Fixes/refactors for NEW features don't count:**
   - If a feature was introduced in this PR, any fix/refactor for it is NOT a fix to main
   - `feat: Add auth` + `fix: Fix auth bug` → Feature: "Add auth", NO fix entry
   - `feat: Add login` + `refactor: Clean up login` → Feature: "Add login", NO refactor entry
   - Only list Fixes/Refactors for code that **already exists on main**

   **Collapse related commits:**
   - Feature + its fixes/refactors = single feature entry (no separate fix/refactor)
   - The fix is part of implementing the feature, not a deliverable

   **Omit superseded changes:**
   - `refactor: Extract helper` + `refactor: Rewrite helper` → just `Rewrite helper`
   - `feat: Add config v1` + `feat: Replace config with v2` → just `Add config` (final state)
   - Earlier approach replaced by later = only mention final

   **Omit reverted changes:**
   - `feat: Add X` + `revert: Remove X` → omit both
   - Net zero changes don't appear

   **Use diff to verify:**
   - Check `git diff --stat` to see what files actually changed
   - If a file was added then deleted, it's not in the diff = omit
   - Final state matters, not intermediate states

5. **Generate PR title** - concise, describes the main deliverable(s)

6. **Generate summary of net effect**

## Output Format

```markdown
## PR Title
Add user authentication and password reset

## Summary

### Features
- User authentication with JWT tokens
- Password reset flow

### Fixes
- Session timeout on mobile browsers (existing issue)

### Refactors
- Simplify database connection pooling

### Docs
- API authentication guide
```

**Rules:**
- **PR Title**: Concise (50-72 chars), describes main deliverable(s), no prefix
- Show **what the PR delivers**, not commit-by-commit history
- Feature + its fixes/refactors = one feature bullet (no separate entries)
- **Fixes section**: ONLY for bugs in code that exists on main, NOT for bugs in new features
- **Refactors section**: ONLY for refactoring code that exists on main, NOT for new feature code
- Only latest approach if multiple iterations
- Omit anything that nets to zero change
- One bullet per distinct deliverable
- Skip empty sections

## Examples

**Commits:**
```
feat: Add user profile page
fix: Fix profile avatar display
fix: Handle missing profile data
refactor: Extract profile components
fix: Profile mobile layout
```

**Output:**
```markdown
## PR Title
Add user profile page

## Summary

### Features
- User profile page with avatar display
```
(All fixes/refactors were for the new feature - no Fix/Refactor sections needed)

---

**Commits:**
```
feat: Add caching with Redis
refactor: Replace Redis with in-memory cache
fix: Cache invalidation bug
```

**Output:**
```markdown
## PR Title
Add in-memory caching layer

## Summary

### Features
- In-memory caching layer
```
(Redis was replaced, fix was for new feature code)

---

**Commits:**
```
feat: Add dark mode
revert: Remove dark mode
feat: Add notification preferences
```

**Output:**
```markdown
## PR Title
Add notification preferences

## Summary

### Features
- Notification preferences
```
(Dark mode was reverted, net zero)

---

**Commits:**
```
feat: Add search feature
fix: Fix existing login timeout bug
refactor: Simplify existing auth middleware
```

**Output:**
```markdown
## PR Title
Add search and fix login timeout

## Summary

### Features
- Search feature

### Fixes
- Login timeout bug

### Refactors
- Simplify auth middleware
```
(Login and auth middleware exist on main - these ARE real fixes/refactors)

## Warning (main branch)

```
⚠️  You are on main branch with unpushed commits.

Create a feature branch before pushing:
  git checkout -b feature/your-feature
```
