# AGENTS.md

## Commands
- Build the CLI with `go build .`; release builds inject the version with `go build -ldflags "-X main.version=0.1.0" .`.
- Fast local verification: `go vet ./...` then `go test ./...`.
- CI is stricter: `go vet ./...`, `go build ./...`, then `go test -race -coverprofile=coverage.out -covermode=atomic ./...` on Ubuntu and macOS.
- Focus a test with package-scoped `go test`, for example `go test ./cmd/ -run TestCreateRollbackOnAfterCreateFailure` or `go test ./internal/files/ -run TestFindEnvFiles`.
- `go install .` installs the local binary. Do not hand-edit the built `/grove` binary or `go.sum`; regenerate them through Go commands.

## Architecture
- Entry point is `main.go`, which sets `cmd.Version` from ldflags or Go build info, then calls `cmd.Execute()`.
- Commands live in `cmd/`; each subcommand registers itself on `rootCmd` in `init()`. Shared command resolution is in `cmd/helpers.go`.
- Config is committed `.groverc.json`, loaded by `internal/config`. Commands find the project root by walking to `.groverc.json`, with a git-common-dir fallback for sibling worktrees.
- Local machine state is `.grove/state.json`, managed by `internal/state`; missing state means no worktrees and is not an error. `.grove/` must stay uncommitted.
- Git operations should go through `internal/git`, not direct `exec.Command("git", ...)`, unless the command is intentionally outside that wrapper's scope.

## Grove Behavior To Preserve
- `grove create` must roll back `git worktree add` on any setup failure after the worktree exists. Preserve the `setupErr` pattern in `cmd/create.go`; tests assert no orphaned worktree or state entry remains.
- Worktree paths are `worktreeDir` + `prefix` + `-` + alias, resolved to absolute paths and through the parent symlink to handle macOS `/tmp` versus `/private/tmp`.
- `afterCreate` and `afterDetachedCreate` accept a string or array, run sequentially via `sh -c`, fail fast, and expose `GROVE_PORT`, `GROVE_ALIAS`, `GROVE_BRANCH`, and `GROVE_PATH`.
- `beforeRemove` and `afterRemove` accept the same string-or-array hook format for managed worktrees. `beforeRemove` runs in the worktree and can block deletion; `afterRemove` runs from the project root after deletion and state update. Orphan cleanup and stale state cleanup do not run remove hooks.
- Protected worktrees are persisted in state. `grove clean` and `grove prune` skip them, and `grove remove` refuses them unless `--include-protected` is explicitly passed.
- `--detach` skips configured symlinks and runs `afterDetachedCreate` before `afterCreate`; normal `.env*` copying and `copyDirs` still apply.
- The detector is conservative: Docker Compose only pulls images; Vite/Remix/SvelteKit get an `npm install` fallback only when no package-manager signal exists; Makefile suggestions require an explicit `setup:` target.
- `.env*` copying is recursive but intentionally skips `node_modules`, `.git`, `dist`, `.next`, and `build`.
- Ports are deterministic from alias within the configured range, collision-resolved against existing state, and persisted in `.grove/state.json`.
- `grove rename` preserves the assigned port and all state metadata. It moves only worktrees still at the standard path for the old alias; adopted/custom paths stay in place, and a failed state save rolls a directory move back.

## Orphans And PRs
- An orphan is a git worktree not tracked in Grove state. `grove list` shows it as `?`; `grove adopt`, `grove clean`, `grove doctor`, and `grove remove` all have orphan-specific behavior in `cmd/helpers.go`.
- `resolveWorktree()` resolves aliases, list indexes, branch names, paths, and picker selections. Numeric-only aliases are reserved for `grove cd 3` style indexes.
- `grove review` depends on the `gh` CLI, fetches PR branch metadata, then reuses the normal create-worktree setup path.

## Embedded Skill
- `skills/grove/SKILL.md` is embedded by root-level `skill_embed.go` into `cmd.SkillContent`; keep the embed file at repo root because Go embed cannot ascend with `..`.
- `grove skill install|uninstall|path` autodetects `~/.claude` and `~/.agents`; flags include `--target`, `--dir`, and `--force`.

## Repo Hygiene
- `CLAUDE.md`, `.reports/`, `.grove/`, and the built `/grove` binary are ignored as local or generated artifacts. Do not copy their prose into this file unless verified against code or CI.
- `.claude/settings.json` is tracked and currently blocks hand edits to `go.sum` and `/grove`, formats edited Go files with `gofmt`, and runs `go vet ./...` after Go edits for Claude Code sessions.
