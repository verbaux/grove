# Spec: Per-worktree notes and v0.9.0 release

## Objective

Let users attach one short local note to each Grove-managed worktree and see it
in daily/list output. Finish the current doctor/state work as a backward-
compatible v0.9.0 release, with the site version updated automatically from the
published release tag.

## CLI and data contract

- `.grove/state.json` gains an optional `note` string on each worktree entry.
  Existing state files remain valid; empty notes are omitted from JSON.
- `grove note <name-or-number>` prints the current note.
- `grove note <name-or-number> <text>` sets/replaces the note transactionally.
- `grove note <name-or-number> --clear` clears the note transactionally.
- Notes are trimmed, single-line UTF-8 text with a maximum of 200 Unicode code
  points. Empty text is rejected in favor of `--clear`.
- Main and orphan worktrees are refused; alias/index/branch/path resolution
  follows the existing managed-worktree resolver.
- Rename preserves the note because it is metadata on `WorktreeEntry`.
- `grove list --json` and `grove status --json` add optional `note` fields.
- Human `grove list` and `grove status` add a `NOTE` column. Plain list output
  remains aliases-only and unchanged.
- The empty-status orphan hint names `grove doctor --fix`, not read-only
  `grove doctor`.

## Release and site contract

- Release version is `v0.9.0`: all changes since v0.8.0 are additive.
- CHANGELOG moves current Unreleased entries under `0.9.0` dated 2026-07-13
  and leaves a fresh empty Unreleased section.
- ROADMAP reports v0.9.0 and moves per-worktree notes to Recently shipped.
- README, embedded skill, AGENTS.md, public site, and local ARCHITECTURE.md
  describe notes and the completed doctor behavior.
- The public site keeps its runtime GitHub latest-release lookup. The deploy
  workflow also runs on a published release and stamps the release tag into the
  deployed HTML, so the displayed fallback does not depend on API availability.
- A `v0.9.0` tag triggers the existing GoReleaser workflow. Release/deploy
  workflows must complete successfully before the task is done.

## Commands

- Focus: `go test ./internal/state ./cmd -run 'Test(SetNote|Note|List.*Note|Status.*Note|StatusHumanNoManaged)'`
- Fast: `go vet ./...` then `go test ./...`
- CI: `go vet ./...`, `go build ./...`, then
  `go test -race -coverprofile=/tmp/grove-v0.9.0-coverage.out -covermode=atomic ./...`
- Release: push the completed branch/main history, tag `v0.9.0`, push the tag,
  then inspect GitHub Actions and the published release.

## Project structure and style

- State behavior stays in `internal/state`; command orchestration stays in
  `cmd/note.go`; list/status reuse `WorktreeEntry.Note` without a parallel store.
- All state mutations use `state.Update` and revalidate alias, branch, and path
  under the lock.
- Public JSON changes are additive and use `omitempty`.
- No new dependencies.

## Testing strategy

- State unit tests prove set/clear/persistence and rename preservation.
- Command integration tests prove set/show/clear, validation, managed-only
  behavior, and concurrent identity revalidation.
- List/status tests prove human and JSON note visibility plus unchanged plain
  output semantics.
- Workflow changes are syntax-reviewed and the release path is verified on
  GitHub after the tag is pushed.
- Site content is checked in a real browser at desktop and mobile widths with a
  clean console.

## Boundaries

- Always: preserve old state compatibility; validate notes before storing;
  update all public/local docs; keep `.codex/` untouched.
- Ask first: changing the chosen public command contract or adding dependencies.
- Never: commit `.grove/` or `ARCHITECTURE.md`; force-push; create the release
  before local review and strict CI pass.

## Success criteria

1. Notes round-trip through state, command, list, status, and JSON.
2. Existing state and plain list consumers remain compatible.
3. The orphan status hint points to `doctor --fix`.
4. Site/docs/skill/roadmap/changelog/architecture are current.
5. v0.9.0 is pushed and published; release and site deploy workflows pass.

## Open questions

None. The command and release contracts above are the selected minimal design.
