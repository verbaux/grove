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
| `beforeRemove` | Hook run before managed worktree removal; failure stops removal |
| `afterRemove` | Hook run after managed worktree removal and state update |
| `portRange` | `{min, max}` for per-worktree port assignment (default 3001–3999) |

Worktree path: `<worktreeDir>/<prefix>-<alias>/`. State in `.grove/state.json` (gitignored).

## Commands

### Create

```sh
grove create feature/my-branch              # alias: "my-branch"
grove create feature/my-branch --name fix   # custom alias
grove create feature/my-branch --from main  # branch from main
grove create feature/my-branch --detach     # skip symlinks; runs afterDetachedCreate before afterCreate
grove create feature/my-branch --json       # machine-readable result
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

### Status

```sh
grove status          # daily read-only overview: dirty, drift, stale paths, symlinks, ports, orphans, freshness
grove status --json   # machine-readable summary and rows
```

Use `grove status` for a quick project overview. Use `grove doctor` when you need a diagnostic checklist with error-level validation.

### Sync

```sh
grove sync my-branch          # apply current config setup to an existing managed worktree
grove sync 2                  # by list index
grove sync my-branch --hooks  # also run afterCreate hooks
grove sync my-branch --json   # machine-readable result
```

Sync is conservative: it copies only missing `.env*` files, creates missing configured symlinks, copies `copyDirs` only when the destination is absent, and updates the stored config hash. It refuses unmanaged/orphan worktrees; run `grove adopt` first.

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
grove doctor          # validates config, config drift, worktree paths, orphans, symlinks, port collisions, gh CLI
grove doctor --json   # machine-readable diagnostics
```

Exits non-zero on errors. Run before manual git-worktree surgery. Also warns on config drift (tracked worktrees created with an older or unknown `.groverc.json` setup), stale `symlink` / `copyDirs` entries (target missing in main repo), and detected project conventions that the config does not yet cover (husky, package managers, direnv, mise, Python, Cargo, Gradle, Next.js, Turbo).

Use JSON output when another tool needs to parse diagnostics.

### Analyze

```sh
grove analyze                          # print suggested .groverc.json additions
grove analyze --clean                  # also list stale symlink/copyDir entries
grove analyze --apply                  # apply additions interactively
grove analyze --apply --clean --yes    # apply additions + remove stale, no prompt
grove analyze --apply --dry-run        # preview resulting config without writing
grove analyze --json                   # machine-readable output
```

Use when adopting Grove on an existing project or after the project gains/loses tooling (e.g. switched from `npm` to `pnpm`, removed `node_modules`, added husky, Docker Compose, Go modules, Bundler, Composer, or Makefile setup). Suggestions already covered by the config are filtered out. Symlink suggestions only fire when the target actually exists in the main repo, so applying them never produces broken symlinks.

Install commands (`yarn install`, `pnpm install`, `uv sync`, `go mod download`, `bundle install`, `composer install`, `cargo fetch`, `./gradlew dependencies`, …) are auto-routed to `afterDetachedCreate` when the corresponding shared directory (`node_modules`, `.venv`, `vendor`, `vendor/bundle`, `target`, `.gradle`) is already in `cfg.symlink`, so they only run with `grove create --detach`. This prevents an install from mutating the main worktree's deps through the symlink.

### Remove

```sh
grove remove my-branch          # checks uncommitted changes
grove remove my-branch --force  # skip check
grove remove my-branch --include-protected  # remove protected worktree too
```

Resolves by alias → branch → path → orphan.

Managed removals run `beforeRemove` in the worktree before deletion and `afterRemove` from the project root after deletion/state update. Orphan cleanup does not run remove hooks.

### Protect

```sh
grove protect my-branch     # prevent accidental removal
grove unprotect my-branch   # allow normal clean/prune/remove again
```

Protected worktrees are skipped by `clean` and `prune`, and `remove` refuses them unless `--include-protected` is passed.

### Clean

```sh
grove clean          # removes ALL managed worktrees, offers to remove orphans
grove clean --force  # skip uncommitted changes check
grove clean --include-protected  # include protected worktrees too
```

### Prune

```sh
grove prune                         # remove merged managed worktrees, prompts first
grove prune --yes                   # non-interactive removal of clean merged worktrees
grove prune --dry-run --json        # machine-readable preview, removes nothing
grove prune --include-protected     # include protected merged worktrees too
```

Use `--dry-run --json` when an agent or script needs to inspect prune candidates before asking the user to approve removal.

## Rules

- NEVER use `git worktree add/remove` directly when Grove is available — bypasses state tracking.
- `grove clean` is destructive — confirm with user before running.
- Protected worktrees are intentional; do not pass `--include-protected` unless the user explicitly asks.
- `grove analyze --apply` and `--apply --clean` mutate `.groverc.json`; show the planned diff (use `--dry-run`) and confirm with the user before running without `--yes`.
- If `beforeRemove` fails, treat it as an intentional block and do not bypass it unless the user explicitly asks.
- When the user asks why hooks/builds/installs aren't running in a worktree, run `grove analyze` (or `grove doctor`) first — most setup gaps are surfaced as detector suggestions or stale-path warnings.
- Reference `$GROVE_PORT` in `afterCreate` / dev commands (stable hash of alias in `portRange`).
