---
name: grove
description: >-
  Use when project has .groverc.json, or user wants isolated branch for a
  feature/bugfix/PR review, or wants to switch branches without stashing.
  Triggers: "create a worktree", "review PR", "switch branch", "grove",
  "isolated branch", "worktree".
---

# Grove — Git Worktree Manager

Use Grove to manage git worktrees instead of raw `git worktree` commands.

## Detection

Check for `.groverc.json` in repo root. If absent, fall back to raw `git worktree` (or suggest `grove init` to user).

Config fields:

| Field | Meaning |
|-------|---------|
| `worktreeDir` | Where to place worktrees (relative to repo root) |
| `prefix` | Prefix for worktree directory names |
| `symlink` | Dirs to symlink from main worktree (e.g. `node_modules`) |
| `copyDirs` | Dirs to copy as build cache (e.g. `.next`, `dist`, `target`) |
| `afterCreate` | Hook command — string OR array (fail-fast sequential) |
| `afterDetachedCreate` | Hook run before `afterCreate` when `--detach` is passed (string OR array) |
| `portRange` | `{min, max}` for per-worktree port assignment (default 3001–3999) |

Worktree path: `<worktreeDir>/<prefix>-<alias>/`. State in `.grove/state.json` (gitignored).

## Commands

### Create

```sh
grove create feature/my-branch              # alias: "my-branch"
grove create feature/my-branch --name fix   # custom alias
grove create feature/my-branch --from main  # branch from main
grove create feature/my-branch --detach     # skip symlinks; runs afterDetachedCreate before afterCreate
```

Creates worktree, copies `.env*`, sets up symlinks, copies `copyDirs`, runs `afterCreate`. Rolls back `git worktree add` on setup failure.

Use `--detach` when branch has different dependencies than main (e.g. major `package.json` bump) — produces a standalone worktree that won't share `node_modules` with main.

`afterCreate` env vars: `$GROVE_PORT`, `$GROVE_ALIAS`, `$GROVE_BRANCH`, `$GROVE_PATH`.

### Switch

```sh
cd $(grove cd my-branch)   # by alias
cd $(grove cd 2)           # by index from grove list
```

NEVER call `grove cd` without arguments from a shell tool — launches interactive picker that hangs.

Combine `cd` with next command in single shell call — `cd` alone has no effect across separate shell invocations:

```sh
cd $(grove cd my-branch) && git status
```

### Review PR

```sh
grove review                    # list open PRs
grove review 42                 # checkout PR #42, alias: "pr-42"
grove review 42 --name hotfix   # custom alias
```

Requires `gh` CLI. Handles fork PRs.

### List

```sh
grove list            # human table (shows port, status)
grove list --json     # parse programmatically
grove list --plain    # aliases only
```

### Adopt orphan

Worktree created via `git worktree add` directly shows as `?` in `grove list`.

```sh
grove adopt                  # auto-selects if only one; picker otherwise
grove adopt feature/legacy   # by branch
```

### Detach

Run from inside a worktree — removes symlinks so worktree becomes independent. Do not run from main repo.

```sh
grove detach          # prompts per symlink whether to copy contents first
grove detach --copy   # copy all symlink targets before removing
```

### Doctor

```sh
grove doctor   # validates config, worktree paths, orphans, symlinks, port collisions, gh CLI
```

Exits non-zero on errors. Run before manual git-worktree surgery.

### Remove

```sh
grove remove my-branch          # checks uncommitted changes
grove remove my-branch --force  # skip check
```

Resolves by alias → branch → path → orphan.

### Clean

```sh
grove clean          # removes ALL managed worktrees, offers to remove orphans
grove clean --force  # skip uncommitted changes check
```

## Rules

- NEVER use `git worktree add/remove` directly when Grove is available — bypasses state tracking.
- `grove clean` is destructive — confirm with user before running.
- Reference `$GROVE_PORT` in `afterCreate` / dev commands (stable hash of alias in `portRange`).
