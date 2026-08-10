# Tasks: Per-worktree notes and v0.9.0

- [x] Correct the empty-status orphan repair hint.
  - Acceptance: human output names `grove doctor --fix`.
  - Verify: focused status test.
  - Files: `cmd/status.go`, `cmd/status_test.go`.
- [x] Add optional note state metadata.
  - Acceptance: set/clear persists; rename preserves notes; old JSON loads.
  - Verify: `go test ./internal/state -run 'Test(SetNote|Rename.*Note|Load)'`.
  - Files: `internal/state/state.go`, `internal/state/state_test.go`.
- [x] Add the transactional `grove note` command.
  - Acceptance: set/show/clear work for managed targets; invalid notes and
    main/orphans are refused.
  - Verify: `go test ./cmd -run TestNote`.
  - Files: `cmd/note.go`, `cmd/note_test.go`, command completion if needed.
- [x] Surface notes in list and status.
  - Acceptance: human and JSON output contain notes; `list --plain` is unchanged.
  - Verify: `go test ./cmd -run 'Test(List.*Note|Status.*Note)'`.
  - Files: `cmd/helpers.go`, `cmd/list.go`, `cmd/status.go`, their tests.
- [x] Update docs and automate site release version.
  - Acceptance: README/skill/site/roadmap/changelog/AGENTS and local architecture
    agree; a successful release calls deploy and passes the tag to stamp.
  - Verify: workflow review, browser desktop/mobile check.
  - Files: documentation, `docs/index.html`, `.github/workflows/deploy-site.yml`.
- [x] Review, verify, publish v0.9.0.
  - Acceptance: strict CI green, branch/main and tag pushed, release/deploy green.
  - Verify: GitHub Actions and release inspection.

## Removal follow-ups

- [x] Add the documented dirty-worktree confirmation to managed `grove remove`.
  - Acceptance: managed and orphan removal use consistent confirmation behavior
    without bypassing submodule, protection, hook, or state safety checks.
- [x] Stop batch removal errors from being printed twice.
  - Acceptance: `grove clean` and `grove prune` report each failed target once
    while still returning a non-zero exit status when any removal fails.
- [x] Preserve the underlying Git error when submodule safety cannot be verified.
  - Acceptance: callers can match `ErrSubmoduleSafetyUnknown` while diagnostics
    retain the original `git submodule status --recursive` failure via wrapping.

## Future removal follow-ups

- [ ] Make confirmation EOF behavior consistent across `remove`, `clean`, and
  `prune`.
  - Current state: `remove` returns a non-zero error with `--force` guidance,
    while `clean` and interactive `prune` treat EOF as the default `n` response
    and exit successfully without removing anything.
- [ ] Consolidate single-target and batch force-decision policy.
  - Current state: `confirmDirtyRemoval` and `forceForRemoval` contain
    overlapping logic, so policy changes must be kept in sync manually.
- [ ] Simplify nested batch-error collection in `runClean`.
  - Current state: append the returned `cleanOrphans` error directly and rely on
    `newBatchRemovalError` to flatten it instead of spreading
    `batchErr.failures` at the call site.
