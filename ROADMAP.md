# Roadmap

Planned direction for Grove. Horizons are priority buckets, not hard dates — items move as needs change. Some items within sections carry ordering dependencies, noted inline. Contributions and suggestions welcome.

## Recently shipped

- **`grove analyze`.** Scans the project for known framework signals and suggests `.groverc.json` additions. Supports `--apply` (write suggestions back), `--dry-run`, `--yes`, `--clean` (remove stale entries), and `--json` for scripting.
- **`afterDetachedCreate` lifecycle hook.** Config field that runs commands only when `grove create --detach` is used — used by the detector to route install commands that would corrupt a shared symlink target.
- **`grove init` convention prompts.** Detected conventions are offered as y/n prompts during `grove init`.
- **`grove open`.** Open a worktree in your editor — resolves by alias/index/picker, then launches `$EDITOR`/`$VISUAL` (or the `editor` config field / `--editor` flag) in the worktree directory.

## Now — toward the 0.1.0 release

- **Ship 0.1.0.** Tag and publish the release. Gate: the CHANGELOG is still marked `Unreleased` and needs updating for `grove analyze`, `--detach` / `afterDetachedCreate`, and the full detector signal set. `goreleaser` and the Homebrew tap config are wired (`brews` stanza → `verbaux/homebrew-tap`), but the remote tap repo and `HOMEBREW_TAP_GITHUB_TOKEN` secret must be in place before publishing succeeds.
- **`grove shell-init` (0.1.1 fast-follow).** Emit the `gcd` shell function and completion setup in one command, replacing the manual paste-a-snippet step. Wraps existing `grove cd` path-printing and the completion infra already in place — ~25 lines. High first-run friction, low effort; ship immediately after 0.1.0.
- **JSON schema for `.groverc.json`.** Publish a schema so editors get validation and autocomplete for the config file. Independent of release mechanics — can ship any time the config struct stabilizes.

## Next — 0.2.x

- **`grove prune`.** Detect worktrees whose branch is already merged and offer to remove them. Requires new git logic to identify merged branches — the existing `doctor` / `clean` machinery handles orphan detection (worktrees git knows but Grove doesn't track) and doesn't cover this case.
- **Detector expansion.** Extends the existing detector (currently ~10 signals across JS, Python, Rust, Gradle, direnv, mise) with Docker / `docker-compose`, Vite, Remix, SvelteKit, Go modules, Ruby/Bundler, PHP/Composer, and `Makefile`.
- **Monorepo awareness.** Handle workspaces so install and symlink suggestions are correct per package, starting with correct symlink suggestions for yarn/pnpm workspaces (multiple `package.json` files, hoisted vs. scoped `node_modules`); later, per-package `afterCreate` targeting and `.env` path resolution across workspace roots.
- **Lifecycle hooks: `beforeRemove` / `afterRemove`.** Add `beforeRemove` and `afterRemove` command fields to `.groverc.json`. Config already has the polymorphic `AfterCreate` type; the same pattern extends cleanly to remove — ~30 lines total, no new dependencies.
- **`grove rename`.** Rename a tracked worktree alias: updates `state.json` and, where needed, calls `git worktree move` to relocate the directory. Needed once aliased worktrees accumulate over time.
- **`grove ps`.** Read-only live view: query `netstat`/`lsof` against the ports tracked in `state.json` and print which dev servers are actually running. Does not change port assignments — that's "Smarter ports" (0.3+).

## Later — 0.3+

- **`grove doctor --fix`.** Auto-repair common issues: adopt orphans, prune stale state entries, fix broken symlinks. The underlying actions (`adopt`, `clean`, state mutation) already ship, but auto-repair mutates the user's state and worktrees — it needs careful design around what gets fixed silently vs. confirmed first, which makes this more than wiring a flag.
- **Smarter ports.** Feed live-process data back into port *assignment* — skip ports that are already bound at create-time — and add support for multiple ports per worktree. (Implement before TUI dashboard — the dashboard reads this enriched port data model.)
- **`grove review` beyond `gh`.** Support GitLab and Bitbucket pull/merge requests.
- **`.env` transform.** Rewrite `PORT=` in copied env files to the assigned `$GROVE_PORT`; add include/exclude patterns for the `.env` walk.
- **TUI dashboard.** Interactive worktree list showing status, port, and inline actions. (Depends on smarter-ports data model for live port status.)

## Maybe / research

- Auto-update the embedded agent skill when `grove` itself is upgraded. (Land this before an MCP server — the update mechanism is shared infrastructure.)
- MCP server for Grove, alongside the existing agent skill. (Builds on the same install/update plumbing as auto-update skill.)
- TTL / auto-expiry for stale worktrees.
- Inter-worktree coordination / sync. Research area: broadcasting state changes (e.g. port assignments, config drift) across active worktrees so long-lived worktrees don't diverge silently.
