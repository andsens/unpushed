# unpushed

A tray icon (StatusNotifierItem) listing your most recently active git
repositories. Each one gets a circle with an up arrow for unpushed commits and
a down arrow for unpulled ones:

- green: no arrows, in sync
- yellow: one arrow, ahead or behind
- red: both arrows (diverged), or the last push or pull failed

The tray icon combines every repository found, not only the listed ones.
Repositories without a remote are left out. The scheme lives in `src/icons.go`:
`Look.Color` picks the colour, `Look.layers` the shapes.

Clicking a yellow or red repo fetches, then pushes branches that are ahead and
fast-forwards branches that are behind. When that fails, or the branch has
diverged, the next click opens a terminal in the repository instead, the one
`xdg-terminal-exec` picks from `xdg-terminals.list`. Nothing
fetches on its own, so a down arrow shows up after you fetch, or for
repositories in `checkRemote`, whose remotes are queried with `git ls-remote`.
That writes nothing to the repository; when the remote has commits you haven't
fetched the count shows as `↓?`.

Repositories are found by a `find` at idle CPU and IO priority. Ref changes
are picked up through inotify on `.git`, `.git/logs` and `.git/refs`.

"Settings…" in the menu opens a window for everything below. It writes only
the keys that differ from their defaults. Adding a repository to the list goes
through the desktop portal's file chooser.

Linux only, built for Wayland.

## Configuration

`~/.config/unpushed.json`, every key optional. The settings window edits it,
so it can't be a file Home Manager manages:

| Key              | Default | Description                                                         |
| ---------------- | ------- | ------------------------------------------------------------------- |
| `roots`          | `["~"]` | Directories to search for repositories                              |
| `depth`          | `5`     | How many levels below a root a repository may be                    |
| `max`            | `5`     | How many repositories to list                                       |
| `repos`          | `[]`    | Repositories to list instead of searching the roots                 |
| `ignore`         | `[]`    | Repositories to leave out of the search, can't be used with `repos` |
| `checkRemote`    | `[]`    | Repositories whose remotes are checked for commits to pull          |
| `scanInterval`   | `"5m"`  | How often to search the roots                                       |
| `remoteInterval` | `"5m"`  | How often to query the remotes in `checkRemote`                     |

## Home Manager

```nix
{
  inputs.unpushed.url = "github:andsens/unpushed";
  # ...
  imports = [ inputs.unpushed.homeModules.default ];
  services.unpushed.enable = true;
}
```
