# Plan: `grove doctor --fix`

1. Define the additive CLI/JSON fix contract and outcome model.
2. Add locked stale-state cleanup and post-fix diagnostics.
3. Add conservative broken-symlink repair.
4. Reuse adoption policy for unambiguous orphan repair.
5. Update public docs and local architecture, then run review and strict CI.

Risks are stale observations and partial filesystem repair. Each fixer therefore
rechecks its target immediately before mutation, records failures explicitly,
and leaves unresolved issues for the second diagnostic pass.
