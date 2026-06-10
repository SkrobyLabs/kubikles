---
name: kubikles-ref
description: Get Kubikles codebase reference. Use when working on Kubikles to understand project structure, patterns, or locate files without scanning the entire codebase.
allowed-tools: Read, Glob
user-invocable: true
---

# Kubikles Complete Reference

`docs/ai/README.md` is the single source of truth for the project structure
and file locator. It is loaded below in full:

!`cat docs/ai/README.md`

> **Required**: When adding/removing/renaming files in `pkg/k8s/`,
> `frontend/src/features/`, `frontend/src/hooks/`, `frontend/src/context/`, or
> root `.go` files, update `docs/ai/README.md` in the same session. The rule
> file (`.claude/rules/kubikles-context.md`) and this skill reference that doc
> rather than duplicating it, so there is only one index to maintain.
>
> **DO NOT** store line counts, file sizes, file counts, or any metrics that
> become stale after code changes.
