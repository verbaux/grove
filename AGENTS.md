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
- Local machine state is `.grove/state.json`, managed by `internal/state`; missing state means no worktrees and is not an error. Production mutations must use `state.Update` (usually through `updateManagedState`) so the `.grove/state.lock` covers a fresh read-modify-write transaction. `.grove/` must stay uncommitted.
- Git operations should go through `internal/git`, not direct `exec.Command("git", ...)`, unless the command is intentionally outside that wrapper's scope.

## Grove Behavior To Preserve
- `grove create` must roll back `git worktree add` on any setup failure after the worktree exists. Preserve the `setupErr` pattern in `cmd/create.go`; tests assert no orphaned worktree or state entry remains. Rollback deletes the local branch only when `git.AddWorktree` reports this run created it; pre-existing branches survive.
- A branch missing locally but present as a remote-tracking branch is created from it with `--track` (`origin` preferred, error with a `--from` hint when several other remotes match); otherwise from `--from` or HEAD.
- Worktree paths are `worktreeDir` + `prefix` + `-` + alias, resolved to absolute paths and through the parent symlink to handle macOS `/tmp` versus `/private/tmp`.
- `afterCreate` and `afterDetachedCreate` accept a string or array, run sequentially via `sh -c`, fail fast, and expose `GROVE_PORT`, `GROVE_ALIAS`, `GROVE_BRANCH`, and `GROVE_PATH`.
- `beforeRemove` and `afterRemove` accept the same string-or-array hook format for managed worktrees. `beforeRemove` runs in the worktree and can block deletion; `afterRemove` runs from the project root after deletion and state update. Orphan cleanup and stale state cleanup do not run remove hooks.
- Protected worktrees are persisted in state. `grove clean` and `grove prune` skip them, and `grove remove` refuses them unless `--include-protected` is explicitly passed.
- `removeManagedWorktree` rechecks the entry (path, branch, protection) from fresh state, runs `git worktree remove` outside the state lock, then drops the entry in a short `state.Update` only if it still matches. Keep slow git and filesystem work out of state transactions; the lock times out after 10s for other processes.
- `--delete-branch` deletes a branch only after its worktree was removed. `grove remove` uses `git branch -d` (`-D` only with `--force`) and keeps unmerged branches with a warning; `grove prune` uses `-D` because `IsMerged` already confirmed the merge, including squash merges. The default branch is never deleted.
- `--detach` skips configured symlinks and runs `afterDetachedCreate` before `afterCreate`; normal `.env*` copying still applies, and `copyDirs` apply unless `copyDirsOnDetach` is `false`. Detached mode is persisted in state, including after `grove detach`, and `grove sync` must preserve it unless `--reattach` completes without real destination conflicts.
- The detector is conservative: Docker Compose only pulls images; Vite/Remix/SvelteKit get an `npm install` fallback only when no package-manager signal exists; Makefile suggestions require an explicit `setup:` target.
- `.env*` copying is recursive but intentionally skips `node_modules`, `.git`, `dist`, `.next`, `build`, and linked worktrees nested under the root (their `.git` file points into `worktrees/`). Copies keep source permission bits; `copyDirs` recreate symlinks as symlinks.
- Ports are deterministic from alias within the configured range, collision-resolved against existing state, and persisted in `.grove/state.json`.
- State writes are atomic, and mutating commands reread state while holding the inter-process lock. Do not reintroduce production `state.Load` → mutate → `state.Save` sequences; they lose unrelated concurrent updates.
- `grove rename` preserves the assigned port and all state metadata. It moves only worktrees still at the standard path for the old alias; adopted/custom paths stay in place, and a failed state save rolls a directory move back.
- Worktree notes are optional local state, limited by `grove note` to one line and 200 Unicode characters. Rename preserves them; `grove list --json` and `grove status --json` omit the `note` field when it is empty.
- `grove exec` requires `--`, chdirs into the worktree before resolving the command, replaces the process via `syscall.Exec` (no shell), and replaces inherited `GROVE_PATH`/`GROVE_BRANCH`/`GROVE_ALIAS`/`GROVE_PORT`; alias and port are set only for managed worktrees.
- `grove ps` is read-only: it scans TCP listeners once with `lsof`, falls back to `netstat`, and never changes ports or state. `netstat` results may lack PID/process details.
- `grove doctor` and `grove doctor --json` are read-only. `grove doctor --fix` may remove revalidated stale state entries, rewrite a tracked path to git's canonical path only for the same branch when no other entry claims it (repairs run before orphan adoption), repair an existing broken configured symlink only when its main-worktree target exists, and adopt only orphans whose branch-derived alias is valid, available, and unique. It must never remove worktrees, overwrite real destinations, modify config, run hooks, or repair port assignments; diagnostics run again after repairs.

## Orphans And PRs
- An orphan is a git worktree not tracked in Grove state. `grove list` shows it as `?`; `grove adopt`, `grove clean`, `grove doctor`, and `grove remove` all have orphan-specific behavior in `cmd/helpers.go`.
- `resolveWorktree()` resolves in this order: picker (empty query), list index, alias, branch in state, path in state, orphan branch or path. Path queries are normalized (absolute, cleaned, symlinks resolved) before comparison. Numeric-only aliases are reserved for `grove cd 3` style indexes.
- `grove review` depends on the `gh` CLI, fetches PR branch metadata, then reuses the normal create-worktree setup path. Fork PRs use a dedicated `pr-<number>` branch; an existing same-repo PR branch is only fast-forwarded, never reset, and left alone when checked out elsewhere.

## Embedded Skill
- `skills/grove/SKILL.md` is embedded by root-level `skill_embed.go` into `cmd.SkillContent`; keep the embed file at repo root because Go embed cannot ascend with `..`.
- `grove skill install|uninstall|path` autodetects `~/.claude` and `~/.agents`; flags include `--target`, `--dir`, and `--force`.

## Repo Hygiene
- `CLAUDE.md`, `.reports/`, `.grove/`, and the built `/grove` binary are ignored as local or generated artifacts. Do not copy their prose into this file unless verified against code or CI.
- `.claude/settings.json` is tracked and currently blocks hand edits to `go.sum` and `/grove`, formats edited Go files with `gofmt`, and runs `go vet ./...` after Go edits for Claude Code sessions.
