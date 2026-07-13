# Plan: Per-worktree notes and v0.9.0

1. Fix the stale `grove status` orphan hint with a regression test.
2. Add the optional note state contract and persistence helpers.
3. Add the transactional `grove note` command and validation.
4. Surface notes in list/status human and JSON output.
5. Update public docs, embedded skill, roadmap, changelog, site, and local
   architecture.
6. Trigger site deployment from release publication and stamp the release tag.
7. Review, run strict CI/browser QA, push, tag v0.9.0, and verify GitHub release
   plus site deployment.

Each numbered item is independently tested and committed before the next one.
