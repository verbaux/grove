# Changelog

## [0.1.0] — Unreleased

First public release.

### Added

- `grove init` — interactive project setup wizard, creates `.groverc.json`
- `grove create <branch>` — create worktree with automatic `.env` copying, symlinks, and post-create hooks; rollback on failure
- `grove remove <name>` — remove worktree by alias, branch, path, or orphan name; checks for uncommitted changes
- `grove list` — show all worktrees with branch, path, and dirty status; `--json` for machine-readable output
- `grove cd <name>` — print worktree path for shell `cd`; supports index numbers and interactive fuzzy picker
- `grove clean` — remove all managed worktrees with optional orphan cleanup
- `grove adopt` — register orphan worktrees (created outside Grove) into state
- `grove review <PR>` — checkout a GitHub PR into a new worktree
- Shell completion for zsh, bash, and fish with dynamic alias suggestions
- Auto-detect version from git tags via `debug.BuildInfo`
- `FindRoot` fallback via `git rev-parse --git-common-dir` for commands run inside worktrees
