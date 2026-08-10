# Roadmap

Planned direction for Grove. Horizons are priority buckets, not hard dates — items move as needs change. Some items within sections carry ordering dependencies, noted inline. Contributions and suggestions welcome.

## Current status

Latest published tag: `v0.10.0`. This release adds glob-based setup paths, persistent detached-worktree policy with safe reattachment, and more reliable removal diagnostics.

## Recently shipped

- **Glob-based setup paths.** `symlink` and `copyDirs` accept root-relative Go glob patterns with deterministic expansion, deduplication, and actionable warnings.
- **Persistent detached setup.** `copyDirsOnDetach` controls build-cache reuse, detached mode is visible in list/status/JSON, and `grove sync --reattach` restores sharing only after local destination conflicts are resolved.
- **Reliable removal batches.** `remove`, `clean`, and `prune` retain per-target diagnostics, aggregate failures without duplicate output, and report partial orphan-cleanup success.
- **Safe submodule removal.** `remove`, `clean`, and `prune` require explicit `--force` before discarding initialized or retained submodule data, and confirmation for one dirty worktree no longer forces clean neighbors.
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
- **Detector expansion.** Added conservative suggestions for Docker Compose, Vite, Remix, SvelteKit, Go modules, Ruby/Bundler, PHP/Composer, and explicit `Makefile` `setup` targets.
- **Config drift detection.** `grove create` records the setup hash for `.groverc.json`; `grove doctor` warns when tracked worktrees were created with an older or unknown config setup.
- **`grove status`.** Read-only daily status view covering dirty worktrees, stale paths, config drift, symlink issues, port collisions, orphans, and branch freshness.
- **`grove sync`.** Updates an existing managed worktree for the current `.groverc.json` setup: missing env files, symlinks, new `copyDirs`, optional `afterCreate` hooks, and refreshed config hash.
- **`grove rename`.** Renames a tracked worktree alias while preserving its port and metadata. Standard Grove paths move with rollback on state-save failure; adopted/custom paths stay in place.
- **`grove ps`.** Shows which assigned worktree ports have live TCP listeners, including PID/process details from `lsof`, a `netstat` fallback, and stable JSON output. It does not change port assignments.
- **Concurrent-safe local state.** Mutating commands serialize `.grove/state.json` transactions through an advisory `.grove/state.lock`, reread the latest state before changing it, and preserve unrelated updates from parallel Grove processes.
- **`grove doctor --fix`.** Explicit, non-interactive repair mode removes revalidated stale state entries, repairs broken configured symlinks when the canonical target exists, and adopts only orphans with a valid, available, unique default alias. It never deletes worktrees and reruns diagnostics afterward.
- **Per-worktree notes.** `grove note` stores short local context for managed worktrees, preserves it across rename, and exposes it in human and JSON list/status output.
- **Release-coupled site version.** A successful GoReleaser job calls the reusable site deploy with the release tag, which stamps both the HTML fallback and CSS cache-buster.

## Later

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
