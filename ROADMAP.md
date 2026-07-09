# Roadmap

Planned direction for Grove. Horizons are priority buckets, not hard dates — items move as needs change. Some items within sections carry ordering dependencies, noted inline. Contributions and suggestions welcome.

## Current status

Latest published tag: `v0.7.2`. See `CHANGELOG` `[Unreleased]` for changes since the latest tag.

## Recently shipped

- **`grove remove` by index.** `grove remove 2` now matches `grove cd` and `grove open`; removing the main worktree index is explicitly refused.
- **Safer `grove prune`.** Prune no longer treats unstarted or behind branches as merged. It still detects merge commits plus squash/rebase merges by content.
- **`grove prune`.** Removes managed worktrees whose branch is already merged into the local base branch. Flags: `--base`, `--yes`, and `--force`.
- **`grove analyze`.** Scans for framework signals and suggests `.groverc.json` additions. Supports `--apply`, `--dry-run`, `--yes`, `--clean`, and `--json`.
- **Detached worktree setup.** `grove create --detach` skips symlinks, and `afterDetachedCreate` runs before `afterCreate` for per-worktree dependency installs.
- **Project convention detector.** `grove init` offers detected conventions as prompts; `grove doctor` warns about missing suggestions and stale configured paths.
- **`grove open` and `grove shell-init`.** Worktree opening now resolves aliases/indexes/picker selections, and shell integration prints completion plus a `gcd` helper for common shells.
- **JSON schema for `.groverc.json`.** `grove init` writes a version-pinned `$schema` URL; dev builds fall back to `main`.
- **Machine-readable output.** Added stable JSON to `grove doctor --json`, `grove create --json`, and `grove prune --dry-run --json`.
- **Lifecycle hooks: `beforeRemove` / `afterRemove`.** Remove-time hooks now run for managed worktrees removed by `grove remove`, `grove clean`, and `grove prune`.
- **`grove protect` / protected worktrees.** Protected worktrees are skipped by `grove clean` and `grove prune`, and refused by `grove remove` unless `--include-protected` is passed.

## Next

- **Detector expansion.** Extend the existing detector with Docker / `docker-compose`, Vite, Remix, SvelteKit, Go modules, Ruby/Bundler, PHP/Composer, and `Makefile`.
- **Config drift detection.** Store the config version or hash used at worktree creation so Grove can report worktrees created before newer symlinks, copy dirs, or hooks were added.
- **`grove status`.** Add a daily status view broader than `grove list`: dirty summary, stale state, broken symlinks, config drift, port status, and branch freshness hints.
- **`grove sync`.** Bring an existing worktree up to the current `.groverc.json`: apply missing symlinks, refresh copied env files where safe, copy new `copyDirs`, and optionally run newly added hooks.
- **`grove rename`.** Rename a tracked worktree alias: updates `state.json` and, where needed, calls `git worktree move` to relocate the directory. Needed once aliased worktrees accumulate over time.
- **`grove ps`.** Read-only live view: query `netstat`/`lsof` against the ports tracked in `state.json` and print which dev servers are actually running. Does not change port assignments — that's "Smarter ports" (0.3+).

## Later

- **`grove doctor --fix`.** Auto-repair common issues: adopt orphans, prune stale state entries, fix broken symlinks. The underlying actions (`adopt`, `clean`, state mutation) already ship, but auto-repair mutates the user's state and worktrees — it needs careful design around what gets fixed silently vs. confirmed first, which makes this more than wiring a flag.
- **Per-worktree notes.** Let users attach short notes to tracked worktrees and show them in status/list output, e.g. why a long-lived worktree still exists.
- **`grove why <alias>`.** Explain how Grove derived a worktree's path, port, symlinks, hooks, and state entry so setup issues are debuggable without reading config internals.
- **Branch freshness hints.** Report branches that are behind the base branch, have no unique commits, lost their upstream, or were deleted remotely.
- **Monorepo awareness.** Handle workspaces so install and symlink suggestions are correct per package, starting with correct symlink suggestions for yarn/pnpm workspaces (multiple `package.json` files, hoisted vs. scoped `node_modules`); later, per-package `afterCreate` targeting and `.env` path resolution across workspace roots.
- **Smarter ports.** Feed live-process data back into port *assignment* — skip ports that are already bound at create-time — and add support for multiple ports per worktree. (Implement before TUI dashboard — the dashboard reads this enriched port data model.)
- **`grove review` beyond `gh`.** Support GitLab and Bitbucket pull/merge requests.
- **`.env` transform.** Rewrite `PORT=` in copied env files to the assigned `$GROVE_PORT`; add include/exclude patterns for the `.env` walk.
- **TUI dashboard.** Interactive worktree list showing status, port, and inline actions. (Depends on smarter-ports data model for live port status.)

## Maybe / research

- Auto-update the embedded agent skill when `grove` itself is upgraded. (Land this before an MCP server — the update mechanism is shared infrastructure.)
- MCP server for Grove, alongside the existing agent skill. (Builds on the same install/update plumbing as auto-update skill.)
- TTL / auto-expiry for stale worktrees.
- Register `groverc.schema.json` with [SchemaStore](https://www.schemastore.org/) so editors validate `.groverc.json` with no `$schema` line. External PR; depends on a stable hosted schema URL (ideally a tagged release rather than `main`).
- Inter-worktree coordination / sync. Research area: broadcasting state changes (e.g. port assignments, config drift) across active worktrees so long-lived worktrees don't diverge silently.
