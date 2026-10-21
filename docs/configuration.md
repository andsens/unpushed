# Configuration

unpushed reads `unpushed.json` from the user's config directory, or the file
`unpushed --config PATH` names:

| OS      | Path                                                                   |
| ------- | ---------------------------------------------------------------------- |
| Linux   | `$XDG_CONFIG_HOME/unpushed.json`, by default `~/.config/unpushed.json` |
| macOS   | `~/Library/Application Support/unpushed.json`                          |
| Windows | `%AppData%\unpushed.json`                                              |

`unpushed --ro-config PATH` reads a base config from `PATH` as well, and the
user's config applies on top of it. The Home Manager module passes its
settings this way. `roots` and `repos` merge path by path and property by
property, so the user's config can override a single property of a path the
base config sets. Every other key replaces the base config's value, and
`null` removes a key the base config sets.

Every key is optional. The settings window writes only the user's config, and
in it only what differs from the base config and the defaults. A changed
default or base config still applies wherever the user's config doesn't
override it.

| Key              | Default                     | Description                                            |
| ---------------- | --------------------------- | ------------------------------------------------------ |
| `roots`          | `{"~": {"include": true}}`  | Directories to search for repositories, see below      |
| `repos`          | `{}`                        | Settings for single repositories, see below            |
| `depth`          | `5`                         | How many levels below a root a repository may be       |
| `max`            | `5`                         | How many repositories the menu lists                   |
| `scanInterval`   | `"5m"`                      | How often to search the roots                          |
| `remoteInterval` | `"5m"`                      | How often to query the remotes set to `checkRemote`    |

The default `roots` apply only when neither config sets any.

`roots` maps a directory to `{"include": BOOL}`. An included directory is
searched, an excluded one is left out. The most specific path wins: with
`~` included, `~/src` excluded and `~/src/work` included, only `~/src/work`
is searched below `~/src`. `depth` counts from each included root. A root
that is a repository itself is taken as it is, without searching below it.
To watch only a fixed set of repositories, make each of them a root.

`repos` maps a repository to its settings, each `false` when left out:

| Property      | Description                                                          |
| ------------- | -------------------------------------------------------------------- |
| `ignore`      | Leave the repository out, even when pinned                           |
| `checkRemote` | Query the remotes for commits to pull                                |
| `pinned`      | List it before the others. Pins count toward `max`, and when there are more of them, the most recently active win |

Paths may start with `~`. Intervals are durations such as `"30m"` or
`"1h30m"`; the settings window offers steps from one minute to six hours.

## Example

```json
{
  "roots": {
    "~/src": {"include": true},
    "~/src/archive": {"include": false},
    "~/.dotfiles": {"include": true}
  },
  "repos": {
    "~/src/work/api": {"checkRemote": true, "pinned": true}
  },
  "remoteInterval": "15m"
}
```
