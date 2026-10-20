# unpushed

A tray icon (StatusNotifierItem) listing your most recently active git
repositories, each with an LED:

- green: in sync
- yellow: behind its upstream
- red: commits no remote has
- ✗: diverged, upstream gone, or the last push/pull failed

Clicking a red or yellow repo fetches, then pushes branches that are ahead and
fast-forwards branches that are behind. Clicking a ✗ opens a terminal in the
repository. Nothing fetches on its own, so yellow shows up after you fetch,
or for repositories in `checkRemote`, whose remotes are queried with
`git ls-remote` every 5 minutes. That writes nothing to the repository; when
the remote has commits you haven't fetched the count shows as `↓?`.

The tray icon is red when any repository has unpushed commits, else yellow
when any is behind, else green. It covers every repository found, not only the
listed ones.

"Ignore" lists the shown repositories, picking one adds it to `ignore` (or
removes it from `repos`). With `repos` set, "Add repository…" picks a
directory to add through the desktop portal's file chooser.

Repositories are found by a `find` at idle CPU and IO priority every 5
minutes. Ref changes are picked up through inotify on `.git`, `.git/logs` and
`.git/refs`.

Linux only.

## Configuration

`~/.config/unpushed.json`, every key optional. The menu edits it, so it can't
be a file Home Manager manages:

| Key           | Default                    | Description                                                         |
| ------------- | -------------------------- | ------------------------------------------------------------------- |
| `roots`       | `["~"]`                    | Directories to search for repositories                              |
| `depth`       | `5`                        | How many levels below a root a repository may be                    |
| `max`         | `5`                        | How many repositories to list                                       |
| `terminal`    | `["konsole", "--workdir"]` | Command opening a terminal, the repository path is appended         |
| `repos`       | `[]`                       | Repositories to list instead of searching the roots                 |
| `ignore`      | `[]`                       | Repositories to leave out of the search, can't be used with `repos` |
| `checkRemote` | `[]`                       | Repositories whose remotes are checked for commits to pull          |

## Home Manager

```nix
{
  inputs.unpushed.url = "github:andsens/unpushed";
  # ...
  imports = [ inputs.unpushed.homeModules.default ];
  services.unpushed.enable = true;
}
```
