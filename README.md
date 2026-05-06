# Grove

Git worktree manager that handles the setup work so you don't have to.

`git worktree add` is fast. The friction is everything after: copying `.env` files, symlinking `node_modules`, running install scripts. Grove automates all of that.

```
$ grove create feature/auth
Creating worktree for branch "feature/auth" at /home/dev/myapp-auth
  ✓ git worktree created
  ✓ copied 2 .env file(s)
  ✓ symlinked node_modules
  ✓ afterCreate done

Worktree "auth" ready (port 3487).
  cd $(grove cd auth)
```

## Why worktrees?

With `git worktree` you can have multiple branches checked out simultaneously in separate directories. No stashing, no context switching — just `cd` to a different folder and you're on a different branch, with a completely separate working directory.

The problem: every new worktree needs `.env` files copied over, `node_modules` set up, sometimes a build step run. Do this manually a few times and you'll stop using worktrees.

Grove does the setup automatically.

## Installation

### From source

```sh
git clone https://github.com/verbaux/grove
cd grove
go install .
```

Requires Go 1.24+.

### Homebrew

```sh
brew tap verbaux/tap
brew install grove
```

## Quick start

```sh
# 1. Run once in your project root
grove init

# 2. Create a worktree for a branch
grove create feature/auth

# 3. Switch to it
cd $(grove cd auth)

# 4. See all active worktrees
grove list

# 5. Done with the branch? Remove it
grove remove auth
```

## Commands

### `grove init`

Interactive wizard that creates `.groverc.json` in the project root.

```
$ grove init
Prefix for worktree directories [myapp]:
Where to place worktrees [../]:
Directories to symlink (comma-separated) [node_modules]:
Directories to copy as build cache (comma-separated, leave empty for none) []:
Port range for worktrees (format min-max) [3001-3999]:
Command to run after creating worktree (leave empty for none) []: npm install

Created .groverc.json

  Prefix:       myapp
  Worktree dir: ../
  Symlink:      node_modules
  After create: npm install

Next: grove create <branch>
```

---

### `grove create <branch>`

Creates a worktree for a branch and sets it up automatically:

1. Runs `git worktree add`
2. Copies all `.env*` files (recursively, preserving directory structure)
3. Creates symlinks for configured directories
4. Copies build cache directories (`copyDirs`) for a warm start
5. Runs `afterCreate` command if set
6. Saves an alias for easy reference

If the branch doesn't exist, it's created from current HEAD.

**Flags:**

| Flag              | Description                                          |
| ----------------- | ---------------------------------------------------- |
| `--name <alias>`  | Custom alias (default: last segment of branch name)  |
| `--from <branch>` | Create the new branch from this base instead of HEAD |
| `--detach`        | Skip symlinks; run `afterDetachedCreate` before `afterCreate` |

**Examples:**

```sh
# feature/auth → alias "auth", worktree at ../myapp-auth
grove create feature/auth

# Custom alias
grove create feature/payment-redesign --name payments

# Branch from a specific base
grove create feature/auth --from main

# Standalone worktree without symlinks (e.g. branch has different deps)
grove create feature/big-deps-bump --detach
```

If setup fails after the worktree is created, Grove rolls back the `git worktree add` so you're not left with an orphaned directory.

---

### `grove list`

Shows all active worktrees with their status.

```
NAME       BRANCH              PATH                          PORT   STATUS
main       main                /home/dev/myapp               -      ✓ clean
auth       feature/auth        /home/dev/myapp-auth          3487   3 modified
payments   feature/payments    /home/dev/myapp-payments      3214   ✓ clean
```

**Flags:**

| Flag      | Description                                  |
| --------- | -------------------------------------------- |
| `--json`  | Output as JSON array (for scripts and tools) |
| `--plain` | Print only worktree aliases, one per line    |

---

### `grove cd <name>`

Prints the path to a worktree so you can `cd` into it. Supports tab completion for aliases.

```sh
cd $(grove cd auth)
```

Without arguments, opens an interactive fuzzy picker:

```sh
cd $(grove cd)
```

Also accepts an index number from `grove list`:

```sh
cd $(grove cd 3)
```

Add a shell function to make this more convenient:

```sh
# ~/.zshrc or ~/.bashrc
gcd() { cd "$(grove cd "$1")"; }
```

Then just: `gcd auth` or `gcd` for the picker

---

### `grove remove <name>`

Removes a worktree by alias. Checks for uncommitted changes first and asks for confirmation. Supports tab completion for aliases.

```sh
grove remove auth

# Skip the check
grove remove auth --force
```

---

### `grove clean`

Removes all grove-managed worktrees, keeping the main working tree intact.

```sh
grove clean

# Skip uncommitted changes check
grove clean --force
```

---

### `grove detach`

Remove symlinks in the current worktree so it becomes fully independent. Run this from inside a worktree.

```sh
cd $(grove cd auth)
grove detach
```

By default, prompts whether to copy each symlink's contents before removing it. Use `--copy` to copy all without prompting.

```sh
# Copy node_modules (and other symlinked dirs) then remove symlinks
grove detach --copy
```

**Flags:**

| Flag     | Description                           |
| -------- | ------------------------------------- |
| `--copy` | Copy symlink targets before removing  |

Useful when your branch has different dependencies and you need a standalone `node_modules` (or other symlinked directories).

---

### `grove adopt`

Register a worktree that was created outside of Grove (e.g. via `git worktree add` directly). These show as `?` in `grove list`.

```sh
# Auto-selects if only one orphan exists
grove adopt

# Specify by branch name
grove adopt feature/legacy
```

---

### `grove template`

Save and reuse `.groverc.json` configurations across projects.

Templates live in `$XDG_CONFIG_HOME/grove/templates/` (or `~/.config/grove/templates/`).

```sh
# In a configured project, save the current config as a template
grove template save nextjs

# List saved templates
grove template list

# Show a template's contents
grove template show nextjs

# Apply a template to the current directory
cd new-project
grove template apply nextjs

# Or initialize from a template (skips the interactive wizard)
grove init --template nextjs

# Delete
grove template delete nextjs
```

Template names: letters, digits, dash, underscore.

---

### `grove doctor`

Diagnose grove configuration and worktree health. Reports problems as `✓`, `⚠`, or `✗` and exits non-zero if any errors are found.

Checks:
- `.groverc.json` is valid and `portRange` is sane
- every worktree path in `.grove/state.json` exists on disk
- no orphan worktrees (git knows about them, grove doesn't)
- configured symlinks in each worktree point to real targets
- no port collisions in state
- configured `symlink` and `copyDirs` paths still exist in the main repo (warn on stale entries)
- detected project conventions (husky, package managers, direnv, mise, Python, Cargo, Gradle, …) suggest config additions
- `gh` CLI is installed (needed for `grove review`)

```sh
grove doctor
```

```
  ✓ project root: /home/dev/myapp
  ✓ .groverc.json valid
  ✓ 3 tracked worktree(s), all paths exist
  ⚠ orphan worktree: /home/dev/myapp-legacy (branch feature/legacy) — run 'grove adopt'
  ✓ 6 symlink(s) checked, all healthy
  ✓ no port collisions
  ✓ gh CLI available

Summary: 6 ok, 1 warn, 0 error
```

---

### `grove analyze`

Scan the project for known framework signals and suggest `.groverc.json` additions. Non-mutating by default — review before applying.

```sh
# Print suggested additions (non-mutating)
grove analyze

# Include stale entries (paths in cfg.symlink / cfg.copyDirs that no longer exist in the main repo)
grove analyze --clean

# Apply additions interactively
grove analyze --apply

# Apply additions + remove stale entries, no prompt
grove analyze --apply --clean --yes

# Preview the resulting config without writing
grove analyze --apply --clean --dry-run

# Machine-readable output
grove analyze --json
```

Detected today:

| Signal | Suggestion |
| --- | --- |
| `.husky/` + `.husky/_` (gitignored) | symlink `.husky/_`, or `afterCreate: npx husky` if the runtime dir is missing |
| `package.json` + `node_modules/` | symlink `node_modules` |
| `package.json` `packageManager` field | `afterCreate: <pnpm\|yarn\|bun\|npm> install` |
| `pnpm-lock.yaml` / `yarn.lock` / `bun.lockb` / `package-lock.json` | matching `<pm> install` afterCreate |
| `next.config.*` | copyDir `.next/cache` |
| `turbo.json` | copyDir `.turbo` |
| `.envrc` | `afterCreate: direnv allow` |
| `.mise.toml` / `.tool-versions` | `afterCreate: mise install` |
| `uv.lock` / `poetry.lock` / `Pipfile.lock` / `requirements.txt` | matching Python install command |
| `Cargo.toml` | `afterCreate: cargo fetch` |
| `build.gradle*` + `gradlew` | `afterCreate: ./gradlew --no-daemon dependencies` |

Suggestions already covered by your config are filtered out automatically. `grove init` runs the same detector and offers each suggestion as a y/n prompt during the wizard.

**Symlink-aware install routing.** When the detector finds an install command (e.g. `yarn install`) and the corresponding shared directory is already in `cfg.symlink` (e.g. `node_modules`), the suggestion is routed to `afterDetachedCreate` instead of `afterCreate`. Reason: running an install through a shared symlink would mutate the main worktree's dependencies. Routed install commands only execute when you explicitly request an independent worktree via `grove create --detach`.

---

### `grove review [pr-number]`

Check out a GitHub pull request into a new worktree. Requires the [GitHub CLI](https://cli.github.com) (`gh`).

```sh
# List open PRs
grove review

# Check out PR #42 into a worktree
grove review 42

# Custom alias (default: pr-42)
grove review 42 --name hotfix
```

Grove fetches the PR branch, creates the worktree, and runs the usual setup (`.env` copy, symlinks, `afterCreate`). Works with fork PRs too.

---

## Shell completion

Grove supports tab completion for commands, flags, and worktree aliases.

```sh
# Zsh (current session)
source <(grove completion zsh)

# Zsh (permanent)
grove completion zsh > "${fpath[1]}/_grove"

# Bash
grove completion bash > /etc/bash_completion.d/grove

# Fish
grove completion fish > ~/.config/fish/completions/grove.fish
```

Tab completion works for:
- Subcommands and flags
- Worktree aliases in `cd`, `remove`
- Orphan branch names in `adopt`

## Config

### `.groverc.json` — commit this

Project config, lives in the repo root.

```json
{
  "worktreeDir": "../",
  "prefix": "myapp",
  "symlink": ["node_modules"],
  "copyDirs": [".next", "dist"],
  "afterCreate": "npm install",
  "afterDetachedCreate": "npm ci",
  "portRange": { "min": 3001, "max": 3999 }
}
```

| Field                 | Default            | Description                                           |
| --------------------- | ------------------ | ----------------------------------------------------- |
| `worktreeDir`         | `../`              | Where to place worktrees relative to the project root |
| `prefix`              | folder name        | Prefix for worktree directory names                   |
| `symlink`             | `["node_modules"]` | Directories to symlink from the main worktree         |
| `copyDirs`            | `[]`               | Directories to copy as build cache (e.g. `.next`, `dist`, `target`) |
| `afterCreate`         | `""`               | Shell command(s) to run after setup — string or array (see below) |
| `afterDetachedCreate` | `""`               | Shell command(s) to run before `afterCreate` when `--detach` is passed (string or array) |
| `portRange`           | `3001–3999`        | Port range assigned to each worktree (see Ports below) |

Worktree path formula: `worktreeDir` + `prefix` + `-` + alias
Example: `../` + `myapp` + `-` + `auth` → `../myapp-auth`

`.env*` files are always found and copied automatically — no config needed.

### `.grove/state.json` — don't commit this

Local state that maps aliases to paths. Add `.grove/` to your `.gitignore`.

```
echo '.grove/' >> .gitignore
```

## Project detection

Grove ships with a built-in detector that recognizes common project conventions (husky hooks, package managers, direnv, mise, Python/Cargo/Gradle ecosystems, Next.js, Turbo, …) and proposes matching `.groverc.json` entries.

- **`grove init`** offers detected suggestions as y/n prompts during the wizard.
- **`grove doctor`** warns when a detected suggestion is missing from the config, or when an existing `symlink` / `copyDirs` entry points to a path that no longer exists in the main repo.
- **`grove analyze`** is the dedicated command for inspecting and applying detector output (see above).

The detector is conservative: symlink suggestions only fire when the target actually exists in the main repo, so applying a suggestion will not produce broken symlinks in fresh worktrees.

## How `.env` copying works

Grove walks your project directory recursively and copies every file matching `.env*` — `.env`, `.env.local`, `.env.production`, nested ones in subdirectories, all of it.

Skips: `node_modules/`, `.git/`, `dist/`, `.next/`, `build/`

Directory structure is preserved. If you have `apps/api/.env.local`, the copy lands at `<worktree>/apps/api/.env.local`.

## How symlinks work

Instead of running `npm install` in each worktree (slow), Grove creates a symlink from the new worktree's `node_modules` to the original. Both worktrees share the same `node_modules` on disk.

This works well when the branches have the same dependencies. If a branch changes `package.json` significantly, use `afterCreate: "npm install"` — it will install into the symlink's target, or you can remove the symlink and install fresh.

## afterCreate

Single command (legacy, still supported):

```json
{ "afterCreate": "npm install" }
```

Or an array of commands — run sequentially, fail-fast:

```json
{
  "afterCreate": [
    "npm ci",
    "npm run build",
    "echo \"ready on $GROVE_PORT\""
  ]
}
```

Each command runs in the worktree directory via `sh -c`, so pipes, `&&`, subshells all work. The `$GROVE_*` env vars are available in every command.

## Detached worktrees (`--detach`)

By default, `grove create` symlinks `node_modules` (and any other configured `symlink` directories) to the main worktree to avoid reinstalling. That is great when branches share dependencies, but breaks down when a branch bumps `package.json` significantly — installing into the symlink mutates the main worktree's modules.

`grove create <branch> --detach` creates a fully independent worktree:
- All entries in `symlink` are skipped.
- If `afterDetachedCreate` is configured, its commands run **before** `afterCreate` so you can install dependencies locally.
- `copyDirs`, `.env*` copying, and `afterCreate` behave exactly as in normal mode.

```json
{
  "symlink": ["node_modules"],
  "afterCreate": "npm run build",
  "afterDetachedCreate": "npm ci"
}
```

```sh
grove create feature/big-bump --detach
# → no node_modules symlink
# → npm ci  (afterDetachedCreate)
# → npm run build  (afterCreate)
```

If you have an existing worktree with symlinks and want to make it standalone, use `grove detach` instead.

## Per-worktree ports

Each worktree gets a stable, deterministic port derived from its alias (hashed into `portRange`, default `3001–3999`). Collisions are resolved via linear probing against already-used ports. The assigned port is saved in `.grove/state.json` so it never changes as long as the worktree exists.

Available as `$GROVE_PORT` in `afterCreate`:

```json
{
  "afterCreate": "echo \"Dev server on $GROVE_PORT\" && PORT=$GROVE_PORT npm run dev &"
}
```

Also exposed: `$GROVE_ALIAS`, `$GROVE_BRANCH`, `$GROVE_PATH`.

## Agent skill

Grove ships with an [agent skill](https://agentskills.io/specification) that teaches AI coding agents (Claude Code, Codex CLI, Gemini CLI, and others) to use `grove` instead of raw `git worktree` commands.

The skill is embedded in the `grove` binary. Install it with:

```sh
# Autodetect installed agents (~/.claude, ~/.agents) and install into each
grove skill install

# Install for a specific agent
grove skill install --target claude
grove skill install --target codex

# Install into a custom directory
grove skill install --dir ~/some/skills

# Overwrite existing files without prompting
grove skill install --force

# Show current install paths and status
grove skill path

# Remove
grove skill uninstall
```

After install, the agent will detect `.groverc.json` and route worktree operations through Grove (create, list, review PRs, adopt orphans, etc.).

## License

MIT
