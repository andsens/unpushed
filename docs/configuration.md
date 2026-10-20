# Configuration

unpushed reads `unpushed.json` from the user's config directory:

| OS      | Path                                                                   |
| ------- | ---------------------------------------------------------------------- |
| Linux   | `$XDG_CONFIG_HOME/unpushed.json`, by default `~/.config/unpushed.json` |
| macOS   | `~/Library/Application Support/unpushed.json`                          |
| Windows | `%AppData%\unpushed.json`                                              |

Every key is optional. The settings window writes only the keys that differ
from their defaults, so a changed default still reaches you. Since the window
edits the file, it can't be a read-only file such as one Home Manager links
into place.

| Key              | Default | Description                                                         |
| ---------------- | ------- | ------------------------------------------------------------------- |
| `roots`          | `["~"]` | Directories to search for repositories                              |
| `depth`          | `5`     | How many levels below a root a repository may be                    |
| `max`            | `5`     | How many repositories the menu lists                                |
| `ignore`         | `[]`    | Repositories to leave out                                           |
| `checkRemote`    | `[]`    | Repositories whose remotes are queried for commits to pull          |
| `scanInterval`   | `"5m"`  | How often to search the roots                                       |
| `remoteInterval` | `"5m"`  | How often to query the remotes of the repositories in `checkRemote` |

Paths may start with `~`. Intervals are durations such as `"30m"` or
`"1h30m"`; the settings window offers steps from one minute to six hours.

A root that is a repository itself is taken as it is, without searching below
it. To watch only a fixed set of repositories, make each of them a root.

## Example

```json
{
  "roots": ["~/src", "~/.dotfiles"],
  "ignore": ["~/src/archive/old-site"],
  "checkRemote": ["~/src/work/api"],
  "remoteInterval": "15m"
}
```
