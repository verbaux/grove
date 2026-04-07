package cmd

const groveAgentsSection = `## Grove — Git Worktree Manager

This project uses [Grove](https://github.com/verbaux/grove) to manage git worktrees.
Use Grove commands instead of raw ` + "`git worktree`" + ` commands.

Config fields in ` + "`.groverc.json`" + `: ` + "`worktreeDir`" + ` (path), ` + "`prefix`" + ` (name prefix), ` + "`symlink`" + ` (dirs to symlink from main), ` + "`afterCreate`" + ` (hook command).
Worktree path formula: ` + "`<worktreeDir>/<prefix>-<alias>/`" + `.

### Commands

- ` + "`grove create <branch>`" + ` — create worktree with .env copy, symlinks, and hooks
- ` + "`grove create <branch> --name <alias>`" + ` — custom alias
- ` + "`grove create <branch> --from main`" + ` — branch from specific base
- ` + "`cd $(grove cd <alias>) && <next command>`" + ` — switch to worktree (always combine cd with next command in a single shell call)
- ` + "`grove list`" + ` — show all worktrees
- ` + "`grove list --json`" + ` — machine-readable output
- ` + "`grove review <pr-number>`" + ` — checkout PR into worktree (requires gh CLI)
- ` + "`grove remove <alias>`" + ` — remove worktree
- ` + "`grove detach`" + ` — remove symlinks in current worktree for independent dependencies (run from inside worktree)
- ` + "`grove clean`" + ` — remove all managed worktrees

### Rules

- NEVER use ` + "`git worktree add/remove`" + ` directly — it bypasses Grove state tracking
- NEVER call ` + "`grove cd`" + ` without arguments — it launches an interactive picker that hangs in non-interactive shells
- Use ` + "`grove list --json`" + ` to parse worktree info programmatically
- Use ` + "`grove review`" + ` for PR checkouts — it handles fetch + worktree + setup in one step
- Always ` + "`grove remove`" + ` when done to keep state clean
- ` + "`grove clean`" + ` is destructive (removes ALL worktrees) — confirm with user before running
`
