# Changelog

## [Unreleased]

### Added

- `grove review --json` prints open PRs as JSON; `grove review <number> --json` prints the created worktree (same fields as `grove create --json`) plus `pr` and `fork`, with progress on stderr

### Changed

- The AGENTS.md section written by `grove init` covers the current commands: JSON output modes, `status`, `sync`, `ps`, `doctor`, `prune --dry-run`, remove hooks, and per-worktree ports

### Fixed

- The first worktree created in a not-yet-existing `worktreeDir` behind a symlinked path (such as macOS `/tmp` or `/var`) is now tracked instead of showing up as an orphan `?`
- `grove cd` and `grove open` without arguments exit with a hint to pass an alias or index when stdin is not a terminal, instead of failing to open `/dev/tty` or waiting for input

## [0.11.1] — 2026-10-28

### Fixed

- `grove analyze`, `grove doctor`, and `grove init` now route an install suggestion to `afterDetachedCreate` when its symlink (such as `node_modules`) is suggested or accepted in the same run, so applying both no longer runs the install inside the main worktree's `node_modules`
- `grove clean` and `grove prune` no longer treat a managed worktree whose directory is gone as having uncommitted changes; `prune --yes` now cleans its state entry instead of skipping it
- `.env*` files are copied again when the project root itself is named like a skipped directory, such as `build` or `dist`
- Port ranges above 65535 are invalid: `grove init` rejects them, `grove doctor` reports them, and worktree creation falls back to the default range
- `grove adopt <path>` matches relative paths, trailing slashes, and symlinked parents, and `grove adopt <query>` exits non-zero when nothing matches
- `grove detach` works from dot-directories inside a worktree, such as `.github`
- The `gcd` helper from `grove shell-init fish` and `powershell` stays put when the picker is cancelled or `grove cd` fails; in fish it used to jump to `$HOME`
- Makefile detection reads `GNUmakefile`, `makefile`, and `Makefile` in GNU make's order and ignores `setup:=`-style variable assignments
- `grove create --detach` no longer reports symlink pattern warnings for symlinks it never creates
- `grove template list` and template completion no longer offer file names that the other template commands reject, such as `foo.bak.json`
- The worktree picker and `grove ps` align columns by display width, so colors and wide or non-ASCII names no longer shift them
- `grove doctor` prints diagnostics in a stable order

## [0.11.0] — 2026-10-20

### Added

- `grove remove --delete-branch` deletes the worktree's local branch after removal with `git branch -d`; unmerged branches are kept with a `git branch -D` hint, and `--force` force-deletes them. `grove prune --delete-branch` force-deletes branches of the merged worktrees it removes, and its dry run lists them. The default branch is never deleted

### Changed

- `grove review` now checks out fork PRs into a dedicated local branch `pr-<number>` instead of the fork's head branch name, so a fork PR from `main` can no longer fetch into your local `main`
- `grove list` and index lookups such as `grove cd 3` check worktree status with up to 8 parallel `git status` calls, which makes them several times faster with many worktrees
- `grove remove`, `prune`, and `clean` no longer hold the state lock while git deletes a worktree, so other Grove commands are not blocked or timed out by large removals. Protection is rechecked right before deletion, and a state entry that changed concurrently is kept with `grove doctor` guidance

### Fixed

- `copyDirs` no longer fails on symlinks: links inside copied directories are recreated as links, and a symlinked `copyDirs` entry is copied by content
- Copied `.env*` files and `copyDirs` files now keep their source permissions instead of becoming `0644`, so secrets stay private and executables stay executable
- `grove create` for a branch that exists only on a remote now creates it from the remote branch with upstream tracking instead of starting a fresh branch from HEAD
- `grove create` rollback now deletes the branch when the failed run created it, so a retry does not silently reuse it and ignore `--from`
- `.env*` copying skips other git worktrees nested under the project, so a `worktreeDir` inside the root no longer copies their env files into new worktrees
- `grove review` fast-forwards an existing local branch of a same-repo PR to the fetched head instead of checking out a stale copy; diverged branches are kept with a warning
- `grove review` PR list now measures and truncates columns by display width, so non-ASCII titles stay aligned and valid UTF-8, and sanitizes PR titles, authors, and branch names before printing them
- Commands that take a worktree reference (`remove`, `cd`, `open`, `note`, `protect`, `rename`, …) now match paths however they are spelled: relative (`../myapp-auth`), with a trailing slash, `.` from the worktree root, or through a symlinked parent such as macOS `/tmp`

## [0.10.0] — 2026-08-10

### Added

- `symlink` and `copyDirs` now accept root-relative Go glob patterns, with deterministic deduplication, warnings for unusable matches, and unchanged support for literal file and directory entries
- `copyDirsOnDetach` can disable build-artifact copying for detached worktrees while preserving the existing copy-by-default behavior; detached mode is visible in list/status/sync output, preserved by sync, and reversibly cleared by `grove sync --reattach` only after symlink conflicts are resolved

### Fixed

- `grove remove` now confirms dirty managed worktrees consistently, returns a non-zero error with `--force` guidance when confirmation input is unavailable, and preserves underlying submodule-safety diagnostics
- `grove clean` and `grove prune` now aggregate per-worktree removal failures without duplicate or lost diagnostics, continue safe batch removals after individual failures, and report successful orphan removals during partial failure

## [0.9.1] — 2026-08-09

### Fixed

- Worktree removal now explains when initialized Git submodules require an explicit `--force`, instead of returning Git's raw submodule error
- `grove clean` and `grove prune` now apply confirmation-derived force per worktree, so one dirty worktree cannot force removal of clean neighbors

### Changed

- Release automation now invokes the reusable site deploy after GoReleaser succeeds and passes the tag used for the HTML fallback and CSS cache-buster

## [0.9.0] — 2026-07-13

### Added

- Config drift detection: `grove create` now records the setup hash for `.groverc.json`, and `grove doctor` warns when tracked worktrees were created with an older or unknown config setup
- `grove status` for a read-only daily overview of managed worktrees: dirty state, stale paths, config drift, symlink issues, port collisions, orphan count, and branch freshness
- `grove sync` to bring an existing managed worktree up to the current `.groverc.json` setup by applying missing env files, symlinks, copy dirs, and optionally `afterCreate` hooks
- `grove rename <name-or-number> <new-alias>` to rename managed worktrees while preserving their branch, port, protection, note, creation time, and config hash; standard Grove paths move with rollback on state-save failure, while custom paths stay in place
- `grove ps` for a read-only live view of TCP listeners on managed worktree ports, with PID/process details from `lsof`, a portable `netstat` fallback, and stable `--json` output
- `grove doctor --fix` for non-interactive repair of revalidated stale state entries, broken configured symlinks with an existing canonical target, and orphan worktrees whose default alias is valid, available, and unambiguous; repair mode never removes worktrees and reports a machine-readable `fixes` audit trail with `--json`
- `grove note <name-or-number> [text]` for short local worktree notes that appear in `grove list`, `grove status`, and their JSON output, survive rename, and can be removed with `--clear`

### Changed

- Mutating commands now coordinate `.grove/state.json` updates with an inter-process lock and fresh read-modify-write transactions, preventing parallel Grove processes from silently overwriting one another's state changes

## [0.8.0] — 2026-07-09

### Added

- `grove doctor --json`, `grove create --json`, and `grove prune --dry-run --json` for scripts and agents that need stable machine-readable output
- `beforeRemove` and `afterRemove` config hooks for managed worktrees removed by `grove remove`, `grove clean`, or `grove prune`
- `grove protect` and `grove unprotect`; protected worktrees are skipped by `clean`/`prune` and require `--include-protected` for explicit removal
- Detector coverage for Docker Compose, Vite, Remix, SvelteKit, Go modules, Ruby/Bundler, PHP/Composer, and explicit `Makefile` `setup` targets

## [0.7.2] — 2026-05-26

### Fixed

- `grove remove` now accepts an index number from `grove list` (e.g. `grove remove 2`), matching `grove cd` and `grove open` which already did — previously only an alias, branch, or path worked. Removing the main worktree (index 1) is now explicitly refused instead of producing a raw git error

## [0.7.1] — 2026-05-25

### Fixed

- `grove prune` no longer flags worktrees whose branch has no commits beyond the base branch (unstarted or behind) as merged — these were incorrectly offered for removal. A branch contained in base is now treated as merged only when its tip sits off base's first-parent trunk (a real merge commit); squash and rebase merges are still detected by content

## [0.7.0] — 2026-05-25

### Added

- `grove prune` — remove worktrees whose branch is already merged into the base branch (auto-detected, or `--base <branch>`); detects regular and squash merges; `--yes` skips the prompt, `--force` removes worktrees with uncommitted changes

## [0.6.1] — 2026-05-24

### Changed

- The `$schema` URL written by `grove init` is now pinned to the grove version that wrote it (dev builds fall back to `main`), so config validation matches the binary

## [0.6.0] — 2026-05-24

### Added

- `grove open [name]` — open a worktree in your editor; resolves by alias/index/picker, then launches `$EDITOR`/`$VISUAL` (or the `editor` config field / `--editor` flag) in the worktree directory
- `grove shell-init [shell]` — print shell integration (tab completion + a `gcd` helper) for bash/zsh/fish/powershell to `eval` from a startup file
- `grove analyze` — scan the project for framework signals and suggest `.groverc.json` additions; `--apply`, `--clean`, `--dry-run`, `--yes`, `--json`
- `grove create --detach` and the `afterDetachedCreate` config hook — create a fully independent worktree (no symlinks), running install commands locally instead of mutating the main tree
- Project convention detector — recognizes husky, package managers, direnv, mise, Python, Cargo, Gradle, Next.js, Turbo; offered as prompts in `grove init` and surfaced by `grove doctor`
- Symlink-aware install routing — install suggestions for an already-symlinked dir route to `afterDetachedCreate` so shared dependencies aren't mutated
- Embedded agent skill and `grove skill install|uninstall|path` for Claude / Codex / Gemini
- JSON schema for `.groverc.json` (`groverc.schema.json`); `grove init` writes a `$schema` reference so editors get autocomplete and validation
- `editor` config field for `grove open`

### Changed

- `grove doctor` now warns on stale `symlink`/`copyDirs` paths and missing detector suggestions

## [0.1.0] – [0.5.0]

Released. See the [git tags](https://github.com/verbaux/grove/tags) and [GitHub Releases](https://github.com/verbaux/grove/releases) for per-version details.
