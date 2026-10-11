# Plugins

🇩🇪 [Deutsche Version](plugins.md)

A plugin is any program that adds actions and live displays to the deck.
It runs as a separate process and speaks line-based JSON over
stdin/stdout – the language doesn't matter.

## Installation

Each plugin lives in its own folder below `plugins/` next to `deck.yaml`,
e.g. `~/.config/ulanzi-deck/plugins/weather/`. The editor lists installed
plugins in the action list under "Plugins"; reload the editor after
copying a plugin.

There is an example in [examples/plugins/weather](../examples/plugins/weather).

## plugin.yaml

```yaml
name: Weather                      # display name
command: [python3, weather.py]     # program and arguments; ./prog runs from the plugin folder
actions:
  - id: refresh                    # stored as op in deck.yaml
    label: Refresh weather
    icon: fa:solid:cloud-sun       # optional, icon from the catalogue
    settings:                      # optional: input fields in the editor
      - {key: city, label: City, placeholder: Berlin}
widgets:
  - id: current
    label: Current weather
    settings:
      - {key: city, label: City, placeholder: Berlin}
```

In `deck.yaml`:

```yaml
- index: 3
  action: {type: plugin, plugin: weather, op: refresh}
  live: {type: plugin, plugin: weather, show: current, settings: {city: Kiel}}
```

## Protocol

The deck starts the program on first use, with the plugin folder as
working directory and `ULANZI_DECK_VERSION` in the environment. If the
program exits, it is started again on next use (after 5 s at the
earliest). Every message is a JSON object on one line.

From the deck to the plugin (stdin):

| Message | Meaning |
| ------- | ------- |
| `{"event":"start","version":"v0.5.0"}` | right after the start |
| `{"event":"action","action":"refresh","settings":{…}}` | a key with this action was pressed |
| `{"event":"widget","widget":"current","instance":"current-1a2b…","settings":{…}}` | a live display is needed; once per combination of widget and settings |

From the plugin to the deck (stdout):

| Message | Meaning |
| ------- | ------- |
| `{"update":{"instance":"current-1a2b…","title":"Kiel","lines":["14°C","Rain"],"color":"#1E3A8A"}}` | new content for a live display; `color` (background) is optional, at most 4 lines |
| `{"log":"text"}` | goes to the deck's log |

The plugin sends updates whenever something changes – the deck doesn't
poll. Output on stderr shows up as a warning in the log.
