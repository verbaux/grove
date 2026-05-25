# Changelog

## [Unreleased]

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
