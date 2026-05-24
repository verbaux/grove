# Roadmap

Planned direction for Grove. Horizons are priority buckets, not hard dates — items move as needs change. Some items within sections carry ordering dependencies, noted inline. Contributions and suggestions welcome.

## Recently shipped

- **`grove analyze`.** Scans the project for known framework signals and suggests `.groverc.json` additions. Supports `--apply` (write suggestions back), `--dry-run`, `--yes`, `--clean` (remove stale entries), and `--json` for scripting.
- **`afterDetachedCreate` lifecycle hook.** Config field that runs commands only when `grove create --detach` is used — used by the detector to route install commands that would corrupt a shared symlink target.
- **`grove init` convention prompts.** Detected conventions are offered as y/n prompts during `grove init`.
- **`grove open`.** Open a worktree in your editor — resolves by alias/index/picker, then launches `$EDITOR`/`$VISUAL` (or the `editor` config field / `--editor` flag) in the worktree directory.
- **`grove shell-init`.** Print shell integration (tab completion + a `gcd` helper) for `bash`/`zsh`/`fish`/`powershell` to `eval` from a shell startup file, replacing the manual paste-a-snippet step.
- **JSON schema for `.groverc.json`.** Ships `groverc.schema.json`; `grove init` writes a `$schema` reference so editors get autocomplete, validation, and field descriptions. (Registering the schema with SchemaStore for zero-config editor support is still open — see below.)

## Now — cut the next release

The latest published tag is `v0.5.0`. A batch of features has landed since (agent skill, `--detach`, the detector/recommender, `grove analyze`, `grove open`, `grove shell-init`, the JSON schema). The `CHANGELOG` `[Unreleased]` section lists them.

- **Tag and publish the next release** (`v0.5.1` or `v0.6.0` per semver — these are additive features, so `v0.6.0`). The existing `goreleaser` + Homebrew tap pipeline already produces releases; tagging is the trigger. Move the `[Unreleased]` changelog block under the new version when cutting it.
- **Pin the schema URL to the release tag.** `$schema` currently points at `main` (mutable). Repoint it to the tagged path once `v0.6.0` is out, so editors validate against a stable schema.

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
- Register `groverc.schema.json` with [SchemaStore](https://www.schemastore.org/) so editors validate `.groverc.json` with no `$schema` line. External PR; depends on a stable hosted schema URL (ideally a tagged release rather than `main`).
- Inter-worktree coordination / sync. Research area: broadcasting state changes (e.g. port assignments, config drift) across active worktrees so long-lived worktrees don't diverge silently.
