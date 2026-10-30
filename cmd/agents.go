package cmd

const groveAgentsSection = `## Grove — Git Worktree Manager

This project uses [Grove](https://github.com/verbaux/grove) to manage git worktrees.
Use Grove commands instead of raw ` + "`git worktree`" + ` commands.

Config lives in ` + "`.groverc.json`" + `: ` + "`worktreeDir`" + `, ` + "`prefix`" + `, ` + "`symlink`" + ` (shared from main), ` + "`copyDirs`" + ` (copied build cache), ` + "`afterCreate`" + ` / ` + "`afterDetachedCreate`" + ` / ` + "`beforeRemove`" + ` / ` + "`afterRemove`" + ` (hooks), ` + "`portRange`" + `.
Worktree path formula: ` + "`<worktreeDir>/<prefix>-<alias>/`" + `.

### Commands

- ` + "`grove create <branch> --json`" + ` — create worktree (.env copy, symlinks, hooks); prints alias, path, port. ` + "`--name <alias>`" + `, ` + "`--from <base>`" + `, ` + "`--detach`" + ` (no shared symlinks)
- ` + "`grove review <pr-number> --json`" + ` — check out a PR into a worktree (requires gh CLI); without a number lists open PRs
- ` + "`grove exec <alias-or-index> -- <command>`" + ` — run in a worktree with ` + "`GROVE_PORT`" + ` etc. set (cd does not persist between shell calls; use ` + "`sh -c`" + ` for pipes)
- ` + "`grove list --json`" + ` / ` + "`grove status --json`" + ` — worktrees with path, port, status, note
- ` + "`grove ps --json`" + ` — live listeners on assigned worktree ports (read-only)
- ` + "`grove sync <alias> --json`" + ` — apply current config to an existing worktree
- ` + "`grove note <alias> \"text\"`" + ` — short local note; ` + "`--clear`" + ` removes it
- ` + "`grove rename <alias-or-index> <new-alias>`" + ` — rename a managed worktree; keeps its port
- ` + "`grove detach`" + ` — run inside a worktree to remove shared symlinks; ` + "`--copy`" + ` copies their contents first
- ` + "`grove doctor --json`" + ` — diagnostics; ` + "`--fix`" + ` repairs only safe, unambiguous issues
- ` + "`grove remove <alias>`" + ` — remove worktree; ` + "`--delete-branch`" + ` also deletes a merged branch
- ` + "`grove prune --dry-run --json`" + ` — preview merged worktrees that prune would remove

### Rules

- NEVER use ` + "`git worktree add/remove`" + ` directly — it bypasses Grove state tracking
- Always pass an alias or index to ` + "`grove cd`" + ` / ` + "`grove open`" + `; without one they need an interactive terminal
- Run dev servers on the worktree's own port: ` + "`$GROVE_PORT`" + ` inside ` + "`grove exec`" + ` and hooks, or ` + "`port`" + ` in ` + "`grove list --json`" + `; it is stable per worktree
- Do not pass ` + "`--force`" + `, ` + "`--include-protected`" + `, or run ` + "`grove clean`" + ` / ` + "`grove prune`" + ` without user confirmation
- If ` + "`beforeRemove`" + ` blocks removal, treat it as intentional
`
