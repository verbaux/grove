# Spec: `grove doctor --fix`

## Objective

Add an explicit repair mode to `grove doctor` for common local inconsistencies
without making ordinary diagnostics mutate the project. Success means parallel
Grove use cannot lose state changes, repair never deletes a worktree, and
remaining problems are reported after fixes are applied.

## CLI contract

- `grove doctor` and `grove doctor --json` remain read-only and retain their
  current diagnostics and exit semantics.
- `grove doctor --fix` applies deterministic repairs without prompting:
  - remove state entries whose worktree path is still missing when the locked
    state transaction runs;
  - repair an existing broken configured symlink only when its canonical target
    exists in the main worktree;
  - adopt an orphan only when its branch-derived alias is valid, unused, and
    unique among the current orphans.
- Ambiguous or unsafe orphan aliases are skipped and remain in the final
  diagnostics with the existing `grove adopt` guidance.
- Repair never removes a git worktree, overwrites a real destination, changes
  config, runs lifecycle hooks, or repairs port collisions/config drift.
- Diagnostics run again after repairs. Exit is non-zero when final diagnostics
  or an attempted repair contain an error.
- `grove doctor --fix --json` is non-interactive and adds `fixes` entries with
  stable `action`, `target`, `status`, and `message` fields. Plain
  `grove doctor --json` keeps `fixes` omitted.

## Commands

- Focus: `go test ./cmd -run TestDoctorFix`
- Fast verification: `go vet ./...` then `go test ./...`
- CI verification: `go vet ./...`, `go build ./...`, then
  `go test -race -coverprofile=coverage.out -covermode=atomic ./...`

## Project structure and style

- CLI orchestration and fix reporting stay in `cmd/doctor.go`.
- Existing adoption policy is extracted in `cmd/adopt.go` and reused rather
  than duplicated.
- State mutations use `state.Update`/`updateManagedState`; tests use real temp
  repositories and assert filesystem/state outcomes.
- Fix action names use lower-case kebab case, for example:

```go
doctorFix{Action: "remove-stale-state", Target: alias, Status: "fixed"}
```

## Testing strategy

- Integration tests cover stale cleanup, broken symlink repair, unambiguous
  adoption, ambiguous adoption skip, JSON shape, and post-fix exit semantics.
- Existing doctor tests prove read-only behavior and diagnostics remain intact.
- Race tests cover state transactions in the final CI gate.

## Boundaries

- Always: revalidate filesystem/state immediately before mutation; rerun
  diagnostics after fixes; update `ARCHITECTURE.md` locally.
- Ask first: deleting worktrees, rewriting `.groverc.json`, or adding new
  dependencies.
- Never: infer an alias when multiple orphans derive the same value; run hooks;
  bypass the state transaction; touch the user's untracked `.codex/` files.

## Success criteria

1. All three repair classes have outcome-based tests.
2. Ordinary doctor invocations perform no writes.
3. Fix mode preserves concurrent unrelated state updates.
4. Fix JSON is deterministic and backward-compatible.
5. Full vet/build/race suite passes on the completed branch.

## Open questions

None. Destructive worktree cleanup and config rewrites are explicitly outside
this feature.
