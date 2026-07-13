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
- [ ] Update docs and automate site release version.
  - Acceptance: README/skill/site/roadmap/changelog/AGENTS and local architecture
    agree; deploy runs on published releases and stamps the release tag.
  - Verify: workflow review, browser desktop/mobile check.
  - Files: documentation, `docs/index.html`, `.github/workflows/deploy-site.yml`.
- [ ] Review, verify, publish v0.9.0.
  - Acceptance: strict CI green, branch/main and tag pushed, release/deploy green.
  - Verify: GitHub Actions and release inspection.
