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

Worktree "auth" ready.
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

Coming soon.

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
4. Runs `afterCreate` command if set
5. Saves an alias for easy reference

If the branch doesn't exist, it's created from current HEAD.

**Flags:**

| Flag              | Description                                          |
| ----------------- | ---------------------------------------------------- |
| `--name <alias>`  | Custom alias (default: last segment of branch name)  |
| `--from <branch>` | Create the new branch from this base instead of HEAD |

**Examples:**

```sh
# feature/auth → alias "auth", worktree at ../myapp-auth
grove create feature/auth

# Custom alias
grove create feature/payment-redesign --name payments

# Branch from a specific base
grove create feature/auth --from main
```

If setup fails after the worktree is created, Grove rolls back the `git worktree add` so you're not left with an orphaned directory.

---

### `grove list`

Shows all active worktrees with their status.

```
NAME       BRANCH              PATH                          STATUS
main       main                /home/dev/myapp               ✓ clean
auth       feature/auth        /home/dev/myapp-auth          3 modified
payments   feature/payments    /home/dev/myapp-payments      ✓ clean
```

---

### `grove cd <name>`

Prints the path to a worktree so you can `cd` into it.

```sh
cd $(grove cd auth)
```

Add a shell alias to make this more convenient:

```sh
# ~/.zshrc or ~/.bashrc
gcd() { cd "$(grove cd "$1")"; }
```

Then just: `gcd auth`

---

### `grove remove <name>`

Removes a worktree by alias. Checks for uncommitted changes first and asks for confirmation.

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

## Config

### `.groverc.json` — commit this

Project config, lives in the repo root.

```json
{
  "worktreeDir": "../",
  "prefix": "myapp",
  "symlink": ["node_modules"],
  "afterCreate": "npm install"
}
```

| Field         | Default            | Description                                           |
| ------------- | ------------------ | ----------------------------------------------------- |
| `worktreeDir` | `../`              | Where to place worktrees relative to the project root |
| `prefix`      | folder name        | Prefix for worktree directory names                   |
| `symlink`     | `["node_modules"]` | Directories to symlink from the main worktree         |
| `afterCreate` | `""`               | Shell command to run in the new worktree after setup  |

Worktree path formula: `worktreeDir` + `prefix` + `-` + alias
Example: `../` + `myapp` + `-` + `auth` → `../myapp-auth`

`.env*` files are always found and copied automatically — no config needed.

### `.grove/state.json` — don't commit this

Local state that maps aliases to paths. Add `.grove/` to your `.gitignore`.

```
echo '.grove/' >> .gitignore
```

## How `.env` copying works

Grove walks your project directory recursively and copies every file matching `.env*` — `.env`, `.env.local`, `.env.production`, nested ones in subdirectories, all of it.

Skips: `node_modules/`, `.git/`, `dist/`, `.next/`, `build/`

Directory structure is preserved. If you have `apps/api/.env.local`, the copy lands at `<worktree>/apps/api/.env.local`.

## How symlinks work

Instead of running `npm install` in each worktree (slow), Grove creates a symlink from the new worktree's `node_modules` to the original. Both worktrees share the same `node_modules` on disk.

This works well when the branches have the same dependencies. If a branch changes `package.json` significantly, use `afterCreate: "npm install"` — it will install into the symlink's target, or you can remove the symlink and install fresh.

## License

MIT
