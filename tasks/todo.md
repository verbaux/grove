# Tasks: `grove doctor --fix`

- [x] Add fix result contract and stale-state repair.
  - Acceptance: stale entries are removed transactionally and JSON reports the action.
  - Verify: `go test ./cmd -run 'TestDoctorFix.*Stale'`.
  - Files: `cmd/doctor.go`, `cmd/doctor_test.go`.
- [ ] Repair broken configured symlinks conservatively.
  - Acceptance: broken links are replaced only when the main target exists.
  - Verify: `go test ./cmd -run 'TestDoctorFix.*Symlink'`.
  - Files: `cmd/doctor.go`, `cmd/doctor_test.go`.
- [ ] Adopt only unambiguous orphan worktrees.
  - Acceptance: unique valid defaults are adopted; collisions remain unresolved.
  - Verify: `go test ./cmd -run 'TestDoctorFix.*Orphan'`.
  - Files: `cmd/adopt.go`, `cmd/doctor.go`, their tests.
- [ ] Document, review, and verify.
  - Acceptance: README/roadmap/changelog/skill and local architecture match behavior.
  - Verify: full CI command sequence from `AGENTS.md`.
