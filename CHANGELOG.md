# Changelog

## [Unreleased]

### Added

- Config drift detection: `grove create` now records the setup hash for `.groverc.json`, and `grove doctor` warns when tracked worktrees were created with an older or unknown config setup
- `grove status` for a read-only daily overview of managed worktrees: dirty state, stale paths, config drift, symlink issues, port collisions, orphan count, and branch freshness
- `grove sync` to bring an existing managed worktree up to the current `.groverc.json` setup by applying missing env files, symlinks, copy dirs, and optionally `afterCreate` hooks
- `grove rename <name-or-number> <new-alias>` to rename managed worktrees while preserving their branch, port, protection, creation time, and config hash; standard Grove paths move with rollback on state-save failure, while custom paths stay in place
- `grove ps` for a read-only live view of TCP listeners on managed worktree ports, with PID/process details from `lsof`, a portable `netstat` fallback, and stable `--json` output

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
