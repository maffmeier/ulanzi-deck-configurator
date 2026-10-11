# Ulanzi Deck

🇩🇪 [Deutsche Version](README.md)

[![CI](https://github.com/maffmeier/ulanzi-deck-configurator/actions/workflows/ci.yml/badge.svg)](https://github.com/maffmeier/ulanzi-deck-configurator/actions/workflows/ci.yml)

Control and configuration for the **Ulanzi Stream Controller D200**
(USB `2207:0019`) on Linux, Windows and macOS – a single binary without
dependencies.

- Keys with icons or text, any number of pages, fixed keys on every page
- Actions: run a command, keyboard shortcut, type text, open a website,
  HTTP request/webhook, switch page (including "next"/"previous" page),
  media keys, audio (volume also per application, microphone, output
  device), meeting keys (Teams, Zoom, Discord, Meet), Home Assistant, OBS,
  MQTT and your own plugins
- Multi actions (macros) with delays, toggle keys with two states, folders
  with an automatic back key
- Pages switch automatically with the active application
- Display automation: dim when idle, night mode, off while the screen is
  locked
- Automations: actions at a time of day, on start, on screen lock or when
  an application gets focus
- Several decks at once, with a shared or their own config
- Per-key behaviour: long press as a second action, repeat while held, or
  hold a shortcut down (push-to-talk)
- Info window: clock, firmware CPU/RAM or your own metrics
  (CPU, RAM, GPU, temperature, disk, network, battery)
- Widgets in the info window and as live displays on keys: command output
  (coloured by exit code), history graph, image/slideshow, timer,
  "now playing" with cover art, next calendar event, audio and Teams
  state, MQTT values
- Animated GIF icons, optional visible key presses
- Web editor in the style of the Stream Deck software with drag & drop,
  a built-in icon catalogue (Font Awesome Free + Twemoji), undo, saved
  versions and export/import of pages as profiles
- UI and device texts in English and German (automatic, switchable)
- Tray icon, autostart, automatic reconnect after unplugging
- Changes to `deck.yaml` are applied without a restart

The protocol comes from the community's reverse engineering (see
[Acknowledgements](#acknowledgements)); the YAML format is compatible with
ulanzi-linux.

> **AI-generated:** Code, tests and documentation of this project were
> largely created with AI assistance, under human direction, review and
> testing on the real device. See [Background](#background).

## Installation

[Releases](https://github.com/maffmeier/ulanzi-deck-configurator/releases)
provide (checksums in `SHA256SUMS`):

| System | Package |
| ------ | ------- |
| Debian, Ubuntu | `ulanzi-deck_<version>_amd64.deb` / `_arm64.deb` – `sudo apt install ./ulanzi-deck_*.deb` |
| Fedora, openSUSE | `ulanzi-deck-<version>-1.x86_64.rpm` / `.aarch64.rpm` – `sudo dnf install ./ulanzi-deck-*.rpm` |
| Arch Linux | AUR package `ulanzi-deck-bin` – `yay -S ulanzi-deck-bin` |
| Windows | `ulanzi-deck-<version>-windows-amd64-setup.exe` (no admin rights needed, optional autostart) |
| macOS | `ulanzi-deck-<version>-macos-universal.dmg` – drag *Ulanzi Deck.app* to *Applications* |
| all | plain binaries `ulanzi-deck-<os>-<arch>` – download, make executable, run |

The Linux packages include the udev rule, a menu entry and an icon.

macOS: the app and binaries are not signed. On first launch, confirm via
right-click → *Open* in Finder or remove the quarantine flag:
`xattr -dr com.apple.quarantine "/Applications/Ulanzi Deck.app"`.

With Go installed you can also run:

```bash
go install github.com/maffmeier/ulanzi-deck-configurator/cmd/ulanzi-deck@latest
```

## Usage

```bash
ulanzi-deck                 # control the deck, editor at http://127.0.0.1:8765, tray icon
ulanzi-deck --open          # also open the editor in the browser
ulanzi-deck devices         # list connected decks
ulanzi-deck listen          # show key presses
ulanzi-deck brightness 60   # set brightness
ulanzi-deck push            # upload the layout once
ulanzi-deck autostart on    # start at login (off | status)
```

Starting a second time simply opens the editor of the running instance.

The configuration is stored by default at

| System  | Path |
| ------- | ---- |
| Linux   | `~/.config/ulanzi-deck/deck.yaml` |
| macOS   | `~/Library/Application Support/ulanzi-deck/deck.yaml` |
| Windows | `%AppData%\ulanzi-deck\deck.yaml` |

and can be overridden with `--config`. Logs are also written to the cache
directory (`~/.cache/ulanzi-deck/ulanzi-deck.log` on Linux).

### Keyboard shortcuts

Written like `ctrl+alt+t`, `super+Return`, `Win+Enter`, `cmd+shift+4`,
`XF86AudioPlay`. The editor can also record the combination.

| System  | Implementation |
| ------- | -------------- |
| Linux   | Wayland (sway, Hyprland & other wlroots compositors): built-in virtual keyboard, otherwise `wtype`; X11: `xdotool` |
| Windows | `SendInput`, no dependencies |
| macOS   | `osascript`; ulanzi-deck needs permission under *Privacy & Security → Accessibility* |

On tiling compositors (sway, Hyprland) a command such as
`swaymsg exec alacritty` is often more robust than a simulated shortcut.

### Key behaviour

```yaml
- index: 3
  action: {type: predefined_command, command_id: media_play_pause}
  long_press: {type: predefined_command, command_id: media_next}  # hold for 0.5 s
- index: 4
  action: {type: predefined_command, command_id: audio_volume_up}
  press: repeat     # repeats while the key is held
- index: 5
  action: {type: shortcut, keys: ctrl+shift+m}
  press: hold       # shortcut stays pressed until the key is released
```

With `long_press`, a short press triggers the normal action on release.
`press: hold` only works with shortcuts; on Wayland it needs a wlroots
compositor, GNOME and KDE don't offer the required interface.

### More actions

```yaml
- index: 6                 # type text (Unicode, emoji too)
  action: {type: type_text, text: "Kind regards"}
- index: 7                 # webhook
  action:
    type: http
    method: POST
    url: https://example.com/hook
    headers: {Content-Type: application/json}
    body: '{"event": "deck"}'
- index: 8                 # macro: actions in sequence, delays in ms
  action:
    type: multi
    steps:
      - {type: shortcut, keys: super+2}
      - {type: delay, ms: 300}
      - {type: type_text, text: "git status\n"}
- index: 9                 # toggle key: switches action, icon and label on every press
  label: Mic on
  action: {type: shell, cmd: "pactl set-source-mute @DEFAULT_SOURCE@ 1"}
  toggle:
    label: Mic off
    icon: ~/icons/mic-off.png
    action: {type: shell, cmd: "pactl set-source-mute @DEFAULT_SOURCE@ 0"}
```

### Folders and automatic pages

```yaml
pages:
  main:
    buttons:
      - {index: 0, label: Git, action: {type: switch_page, page: git}}
  git:
    folder: true           # folder: gets a back key, "next page" skips it
    back_index: 4          # position of the back key (default 0)
    buttons: [...]
  gimp:
    apps: [gimp]           # page is shown while GIMP is in the foreground
    buttons: [...]
```

`apps` matches case-insensitively against the app ID or process name of the
active application; the page settings dialog in the editor offers the
recently seen applications. When focus moves to an application without its
own page, the deck returns to the previous page. The active application is
detected on sway, Hyprland, X11, Windows and macOS; GNOME and KDE on Wayland
don't expose it.

### Display automation

```yaml
display:
  dim_after_s: 300         # after 5 min without a key press, dim …
  dim_brightness: 10       # … to 10 %
  off_when_locked: true    # off while the screen is locked
  night: {from: "22:00", to: "07:00", brightness: 20}
```

### Home Assistant and OBS

Credentials are not stored in `deck.yaml` but in a separate `secrets.yaml`
next to it (mode `0600`), which the editor manages and tests under
*Settings → Integrations*:

```yaml
home_assistant: {url: "http://homeassistant.local:8123", token: "<long-lived access token>"}
obs: {url: "ws://127.0.0.1:4455", password: "<obs-websocket password>"}
mqtt: {url: "mqtt://homeassistant.local:1883", username: deck, password: "<password>"}
teams: {token: "<set by Teams when pairing>"}
```

```yaml
- index: 10                # toggle a light, key lights up while it is on
  label: Kitchen
  action: {type: home_assistant, service: light.toggle, entity_id: light.kitchen}
  live: {type: home_assistant, entity_id: light.kitchen}
- index: 11                # start/stop recording and show its state
  action: {type: obs, op: toggle_record}
  live: {type: obs, show: record}           # scene | record | stream
- index: 12
  action: {type: obs, op: switch_scene, target: Camera}
```

OBS operations: `switch_scene`, `toggle_record`, `start_record`,
`stop_record`, `toggle_stream`, `start_stream`, `stop_stream`, `toggle_mute`
(with the source as `target`), `toggle_replay_buffer`, `save_replay`. OBS
needs obs-websocket 5 (built in since OBS 28). Home Assistant widgets show the
state or an attribute (`attribute`) and colour the key on `active_state`
(default `on`).

### Audio

```yaml
- index: 0
  action: {type: audio, op: volume_up, step: 5}            # volume_down, mute
  live: {type: audio, show: volume}
- index: 1                                                 # Spotify only
  action: {type: audio, op: volume_up, target: spotify}
  live: {type: audio, show: volume, target: spotify}
- index: 2                                                 # red while the microphone is muted
  action: {type: audio, op: mic_mute}
  live: {type: audio, show: mic}
- index: 3                                                 # speakers ↔ headset
  action: {type: audio, op: output_next}                   # output_set with target: Headset
  live: {type: audio, show: output}
```

| System  | Implementation |
| ------- | -------------- |
| Linux   | `pactl` (PulseAudio and PipeWire) |
| Windows | Core Audio; per-application volume on x64 only |
| macOS   | AppleScript; no per-application volume, microphone via input level, device switching with [`SwitchAudioSource`](https://github.com/deweller/switchaudio-osx) |

### Meeting keys

```yaml
- index: 4
  action: {type: meeting, app: teams, op: mute}   # camera, hand, blur, leave
  live: {type: meeting, show: mute}               # camera, hand
- index: 5
  action: {type: meeting, app: zoom, op: camera}  # zoom: mute, camera, hand, share, leave
```

Teams is controlled through its local third-party app API, which also
provides the state for the live display: enable it in Teams under
*Settings → Privacy → Manage third-party app API*, then press a Teams key
during a meeting and choose *Allow* in Teams; the token is stored in
`secrets.yaml`. Zoom, Discord (`mute`, `deafen`) and Google Meet (`mute`,
`camera`, `hand`) get their keyboard shortcut – the app has to be in the
foreground, or you make the shortcuts global in Zoom or Discord.

### MQTT

```yaml
- index: 6
  action: {type: mqtt, topic: zigbee2mqtt/lamp/set, body: '{"state":"TOGGLE"}'}
  live: {type: mqtt, topic: zigbee2mqtt/lamp, attribute: state, active_state: "on", ok_color: '#FACC15'}
```

The broker (`mqtt://host:1883`, or `mqtts://` for TLS) with user and
password lives in `secrets.yaml` and is managed in the editor under
*Settings → Integrations*. `retain: true` makes the broker keep the
value; `attribute` reads a field of JSON messages (also nested, like
`sensor.temperature`).

### Calendar

```yaml
- index: 7
  live: {type: calendar, url: "https://calendar.google.com/calendar/ical/…/basic.ics", interval_s: 300}
```

Shows the current or next event with a countdown; the key changes colour
15 minutes before. `https://`, `webcal://` and local files work, recurring
events are expanded. The ICS address from Google (*Secret address in iCal
format*) or Outlook (*Publish calendar*) is secret – don't share
`deck.yaml` publicly.

### Automations

```yaml
triggers:
  - {event: time, at: "18:00", days: [mon, tue, wed, thu, fri], action: {type: switch_page, page: Evening}}
  - {event: app, app: obs, action: {type: obs, op: start_record}}
  - {event: lock, action: {type: audio, op: mic_mute}}
```

Events: `time` (with `at` and optional `days`), `startup`, `connect` (deck
connected), `lock`, `unlock` and `app` (an application containing this
name gets focus). The editor manages them under *Settings → Automations*.

### Animated icons and key presses

Animated GIFs can be uploaded as icons like any other image; the firmware
plays them itself. Longer GIFs are cut after 120 frames, ones larger than
1.5 MB after conversion are shown as a still image. With
`display: {press_feedback: true}` a pressed key is briefly shown darker
and smaller.

### Plugins

Your own actions and live displays can be written as a plugin in any
language: a folder below `plugins/` next to `deck.yaml` with a
`plugin.yaml` and a program that exchanges JSON lines over stdin/stdout.
Protocol and example: [docs/plugins.en.md](docs/plugins.en.md),
[examples/plugins/weather](examples/plugins/weather).

```yaml
- index: 8
  action: {type: plugin, plugin: weather, op: refresh}
  live: {type: plugin, plugin: weather, show: current, settings: {city: Kiel}}
```

### Several decks

Several D200 run at the same time. By default all of them show
`deck.yaml`; a deck with its own config is mapped by its serial number
(`ulanzi-deck devices`):

```yaml
devices:
  02C43A015U3671626: deck-02C43A015U3671626.yaml
```

In the editor, *Settings → General → Decks → Own config* creates the file
as a copy; the switch at the top then selects the deck to edit.

### Versions and profiles

Every save keeps a version in `backups/` next to `deck.yaml` (the last 50
are kept). In the editor, 🕘 restores an older one; Ctrl+Z and
Ctrl+Shift+Z undo changes before saving. A page (page menu → *Export
page*) or the whole deck (*Settings → General*) can be exported with its
icons as a `.deckprofile` and imported on another machine.

### Language

The editor and device texts follow the system language (German, otherwise
English). The editor has a language switch; `language: en` or `language: de`
in `deck.yaml` fixes the texts on the device.

### Widgets

Instead of the firmware display, the app can render the info window itself
and rotate through several widgets; every key can carry a widget as a live
display.

```yaml
small_window:
  enabled: true
  rotate_every_s: 8
  widgets:
    - {type: clock, format: '%H:%M'}
    - {type: command, title: Weather, cmd: "curl -s 'wttr.in/Berlin?format=3'", interval_s: 600}
    - {type: graph, metric: cpu}            # cpu | memory | network
    - {type: image, paths: [~/pictures/a.png, ~/pictures/b.png], interval_s: 5}
    - {type: timer, title: Pomodoro}
    - {type: media}                         # title, artist, cover
pages:
  main:
    buttons:
      - index: 0                            # red while the microphone is muted
        live:
          type: command
          cmd: "pactl get-source-mute @DEFAULT_SOURCE@ | grep -q no && echo MIC"
          ok_color: '#14532D'
          fail_color: '#B91C1C'
      - index: 1                            # start/pause and show a timer
        action: {type: timer, op: toggle, minutes: 25}   # toggle | start | pause | reset
        live: {type: timer}
```

Commands run in the system shell (`sh` or `cmd`) for at most 10 s; the first
four lines of output are shown. "Now playing" uses MPRIS on Linux (Spotify,
Firefox, VLC, …), the System Media Transport Controls on Windows (anything
that shows up in the volume overlay) and Spotify or the Music app on macOS.

## Platform notes

**Linux:** access to `/dev/hidraw*` needs a udev rule (the packages install
it):

```bash
sudo cp packaging/linux/70-ulanzi-deck.rules /etc/udev/rules.d/
sudo udevadm control --reload-rules && sudo udevadm trigger
```

Autostart uses `~/.config/autostart` (GNOME, KDE, XFCE …). On sway & co. add
`exec ulanzi-deck` to the compositor config instead.

**Windows:** the installer works without admin rights in the user profile.
The official Ulanzi Studio software must not run at the same time.

**macOS:** the app has no Dock icon and lives in the menu bar only. For
shortcuts and active-app detection macOS asks for *Accessibility* and
*Automation* permissions. The release binaries have a tray icon. Builds
cross-compiled with `scripts/build.sh` don't (that would need cgo); built on
a Mac (`CGO_ENABLED=1 go build ./cmd/ulanzi-deck`) it is included.

## Development

Everything runs in the Docker container, nothing is installed on the host.

```bash
docker compose run --rm dev go test ./...           # tests
docker compose run --rm dev sh scripts/build.sh     # binaries for all platforms into dist/
docker compose run --rm dev sh scripts/fetch-assets.sh   # re-download embedded fonts/icons
docker compose run --rm dev sh scripts/notices.sh        # refresh the Go module license notices
```

GitHub Actions checks every push (gofmt, vet, tests on Linux, Windows and
macOS, cross build). A `v*` tag builds the release binaries – macOS natively
with cgo so the tray icon is included – and creates a GitHub release with
`.deb`/`.rpm`, Windows installer and macOS `.dmg`:

```bash
git tag v0.5.0 && git push origin v0.5.0
sh packaging/aur/update.sh 0.5.0     # then the AUR PKGBUILD, see packaging/aur/README.md
```

Layout (domain-driven):

```text
cmd/ulanzi-deck/              CLI, startup, wiring
internal/domain/deck/         configuration, keys, actions, rules
internal/application/         daemon (pages, info window, actions, automation), widgets
internal/infrastructure/
  d200/                       HID protocol, ZIP upload, device driver with reconnect
  configfile/                 read/write deck.yaml
  render/                     key and info window graphics, embedded fonts
  catalog/                    icon catalogue (Font Awesome, Twemoji)
  actions/                    commands, shortcuts, URLs per OS
  metrics/                    system metrics
  media/                      "now playing" (MPRIS, SMTC, AppleScript)
  audio/                      volume, microphone, output device per OS
  desktop/                    active application, screen lock
  integrations/               Home Assistant, OBS, Teams, MQTT, secrets.yaml
  calendar/                   ICS calendars
  plugins/                    external plugins (JSON over stdin/stdout)
  autostart/                  autostart per OS
internal/interfaces/
  web/                        HTTP API and editor (Alpine.js)
  tray/                       tray icon
packaging/                    deb/rpm (nfpm), AUR, Windows installer, macOS app
```

## Acknowledgements

This project wouldn't exist without the community's reverse engineering:

- **[marcelobrake/ulanzi-linux](https://github.com/marcelobrake/ulanzi-linux)**
  – unofficial Linux client in Python. Protocol, device driver, ZIP layout
  including firmware workarounds and daemon logic are a Go port of it; the
  `deck.yaml` format is compatible.
- **[redphx/strmdck](https://github.com/redphx/strmdck)** – the original
  Python library that decoded the D200's HID protocol.
- **[redphx/homedeck](https://github.com/redphx/homedeck)** – Home Assistant
  integration based on strmdck, a good reference for rendering icons.
- **[UlanziTechnology/UlanziDeckPlugin-SDK](https://github.com/UlanziTechnology/UlanziDeckPlugin-SDK)**
  – the official plugin SDK of the Windows/Mac software, useful to check
  manifest and icon sizes.
- **[Hackaday](https://hackaday.com/tag/ulanzi-d200/)** – reports showing
  that the device runs Linux 5.10 on a Rockchip RK3308HS with open ADB root.
- **[rafaelmartins/usbhid](https://rafaelmartins.com/p/usbhid/)** – HID in
  pure Go for all three operating systems.

## Background

This project is AI-generated: code, tests, UI and documentation were largely
written by an AI assistant – as a Go port of ulanzi-linux, extended with
Windows and macOS support, a new editor and more actions. Requirements,
decisions, review and the tests on the real D200 came from the maintainer.

The app is tested on the real device on Linux (Wayland/sway). The Windows
and macOS builds compile and are covered by unit tests but haven't been tried
on real hardware yet – feedback and pull requests are welcome.

## License

MIT – see [LICENSE](LICENSE). Protocol and driver logic are a Go port of
[ulanzi-linux](https://github.com/marcelobrake/ulanzi-linux) (MIT); its
notice and the licences of the embedded fonts, icons and Alpine.js are listed
in [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).

## Disclaimer

Unofficial project, not affiliated with Ulanzi or Fuzhou Rockchip
Electronics. "Ulanzi" and "Stream Controller D200" are trademarks of their
respective owners. Use at your own risk.
