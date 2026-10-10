# Ulanzi Deck

🇬🇧 [English version](README.en.md)

[![CI](https://github.com/maffmeier/ulanzi-deck-configurator/actions/workflows/ci.yml/badge.svg)](https://github.com/maffmeier/ulanzi-deck-configurator/actions/workflows/ci.yml)

Steuerung und Konfiguration für den **Ulanzi Stream Controller D200**
(USB `2207:0019`) unter Linux, Windows und macOS – eine einzelne Binary ohne
Abhängigkeiten.

- Tasten mit Icons oder Text, beliebig viele Seiten, feste Tasten auf allen Seiten
- Aktionen: Befehl ausführen, Tastenkürzel, Text tippen, Website öffnen,
  HTTP-Request/Webhook, Seite wechseln (inkl. „nächste“/„vorherige“ Seite),
  Medien- und Lautstärketasten, Home Assistant, OBS
- Mehrfachaktionen (Makros) mit Pausen, Umschalttasten mit zwei Zuständen,
  Ordner mit automatischer Zurück-Taste
- Seiten wechseln automatisch je nach aktiver Anwendung
- Display-Automatik: dimmen nach Inaktivität, Nachtmodus, aus bei
  gesperrtem Bildschirm
- Tastenverhalten pro Taste: langer Druck als Zweitaktion, Wiederholen
  solange gehalten, oder Kürzel gedrückt halten (Push-to-Talk)
- Infofenster: Uhr, CPU/RAM der Firmware oder eigene Messwerte
  (CPU, RAM, GPU, Temperatur, Festplatte, Netzwerk, Akku)
- Widgets im Infofenster und als Live-Anzeige auf Tasten: Befehlsausgabe
  (mit Farbe je Exit-Code), Verlaufsgrafik, Bild/Diashow, Timer,
  „läuft gerade“ mit Cover
- Web-Editor im Stil der Stream-Deck-Software mit Drag & Drop und
  eingebautem Icon-Katalog (Font Awesome Free + Twemoji)
- Oberfläche und Gerätetexte auf Deutsch und Englisch (automatisch, umschaltbar)
- Tray-Icon, Autostart, automatische Wiederverbindung nach dem Abstecken
- Änderungen an der `deck.yaml` werden ohne Neustart übernommen

Das Protokoll stammt aus dem Reverse Engineering der Community (siehe
[Danksagung](#danksagung-und-vorarbeiten)); das YAML-Format ist zu
ulanzi-linux kompatibel.

> **KI-generiert:** Code, Tests und Dokumentation dieses Projekts wurden
> größtenteils mit KI-Unterstützung erstellt, unter menschlicher Anleitung,
> Prüfung und Tests am echten Gerät. Siehe [Entstehung](#entstehung).

## Installation

Unter [Releases](https://github.com/maffmeier/ulanzi-deck-configurator/releases)
gibt es (Prüfsummen in `SHA256SUMS`):

| System | Paket |
| ------ | ----- |
| Debian, Ubuntu | `ulanzi-deck_<version>_amd64.deb` / `_arm64.deb` – `sudo apt install ./ulanzi-deck_*.deb` |
| Fedora, openSUSE | `ulanzi-deck-<version>-1.x86_64.rpm` / `.aarch64.rpm` – `sudo dnf install ./ulanzi-deck-*.rpm` |
| Arch Linux | AUR-Paket `ulanzi-deck-bin` – `yay -S ulanzi-deck-bin` |
| Windows | `ulanzi-deck-<version>-windows-amd64-setup.exe` (ohne Adminrechte, optional mit Autostart) |
| macOS | `ulanzi-deck-<version>-macos-universal.dmg` – *Ulanzi Deck.app* nach *Programme* ziehen |
| alle | einzelne Binaries `ulanzi-deck-<os>-<arch>` – herunterladen, ausführbar machen, starten |

Die Linux-Pakete bringen udev-Regel, Startmenü-Eintrag und Icon mit.

macOS: App und Binaries sind nicht signiert. Beim ersten Start im Finder per
Rechtsklick → *Öffnen* bestätigen oder die Quarantäne entfernen:
`xattr -dr com.apple.quarantine "/Applications/Ulanzi Deck.app"`.

Mit installiertem Go geht es auch direkt:

```bash
go install github.com/maffmeier/ulanzi-deck-configurator/cmd/ulanzi-deck@latest
```

## Benutzung

```bash
ulanzi-deck                 # Deck steuern, Editor auf http://127.0.0.1:8765, Tray-Icon
ulanzi-deck --open          # zusätzlich den Editor im Browser öffnen
ulanzi-deck devices         # angeschlossene Decks auflisten
ulanzi-deck listen          # Tastendrücke anzeigen
ulanzi-deck brightness 60   # Helligkeit setzen
ulanzi-deck push            # Layout einmalig hochladen
ulanzi-deck autostart on    # beim Anmelden starten (off | status)
```

Ein zweiter Start öffnet einfach den Editor der laufenden Instanz.

Die Konfiguration liegt standardmäßig unter

| System  | Pfad |
| ------- | ---- |
| Linux   | `~/.config/ulanzi-deck/deck.yaml` |
| macOS   | `~/Library/Application Support/ulanzi-deck/deck.yaml` |
| Windows | `%AppData%\ulanzi-deck\deck.yaml` |

und lässt sich mit `--config` überschreiben. Logs landen zusätzlich im
Cache-Verzeichnis (`~/.cache/ulanzi-deck/ulanzi-deck.log` unter Linux).

### Tastenkürzel

Schreibweise wie `ctrl+alt+t`, `super+Return`, `Win+Enter`, `cmd+shift+4`,
`XF86AudioPlay`. Im Editor kann die Kombination auch aufgenommen werden.

| System  | Umsetzung |
| ------- | --------- |
| Linux   | Wayland (sway, Hyprland & andere wlroots-Compositors): eingebaute virtuelle Tastatur, sonst `wtype`; X11: `xdotool` |
| Windows | `SendInput`, keine Abhängigkeiten |
| macOS   | `osascript`; ulanzi-deck braucht die Freigabe unter *Datenschutz & Sicherheit → Bedienungshilfen* |

Unter tiling Compositors (sway, Hyprland) ist oft ein Befehl wie
`swaymsg exec alacritty` robuster als ein simuliertes Kürzel.

### Tastenverhalten

```yaml
- index: 3
  action: {type: predefined_command, command_id: media_play_pause}
  long_press: {type: predefined_command, command_id: media_next}  # ab 0,5 s halten
- index: 4
  action: {type: predefined_command, command_id: audio_volume_up}
  press: repeat     # wiederholt, solange die Taste gehalten wird
- index: 5
  action: {type: shortcut, keys: ctrl+shift+m}
  press: hold       # Kürzel bleibt gedrückt, bis die Taste losgelassen wird
```

Mit `long_press` löst ein kurzer Druck die normale Aktion beim Loslassen
aus. `press: hold` funktioniert nur mit Tastenkürzeln; unter Wayland braucht
es einen wlroots-Compositor, GNOME und KDE bieten die nötige Schnittstelle
nicht.

### Weitere Aktionen

```yaml
- index: 6                 # Text tippen (Unicode, auch Emoji)
  action: {type: type_text, text: "Mit freundlichen Grüßen"}
- index: 7                 # Webhook
  action:
    type: http
    method: POST
    url: https://example.com/hook
    headers: {Content-Type: application/json}
    body: '{"event": "deck"}'
- index: 8                 # Makro: Aktionen nacheinander, Pausen in ms
  action:
    type: multi
    steps:
      - {type: shortcut, keys: super+2}
      - {type: delay, ms: 300}
      - {type: type_text, text: "git status\n"}
- index: 9                 # Umschalttaste: wechselt bei jedem Druck Aktion, Icon und Text
  label: Mikro an
  action: {type: shell, cmd: "pactl set-source-mute @DEFAULT_SOURCE@ 1"}
  toggle:
    label: Mikro aus
    icon: ~/icons/mic-off.png
    action: {type: shell, cmd: "pactl set-source-mute @DEFAULT_SOURCE@ 0"}
```

### Ordner und automatische Seiten

```yaml
pages:
  main:
    buttons:
      - {index: 0, label: Git, action: {type: switch_page, page: git}}
  git:
    folder: true           # Ordner: eigene Zurück-Taste, „nächste Seite“ überspringt ihn
    back_index: 4          # Position der Zurück-Taste (Standard 0)
    buttons: [...]
  gimp:
    apps: [gimp]           # Seite erscheint, solange GIMP im Vordergrund ist
    buttons: [...]
```

`apps` vergleicht ohne Groß-/Kleinschreibung mit der App-ID bzw. dem
Prozessnamen der aktiven Anwendung; der Seiteneinstellungs-Dialog im Editor
zeigt die zuletzt gesehenen Anwendungen zur Auswahl. Wechselt der Fokus zu
einer Anwendung ohne eigene Seite, kehrt das Deck zur vorherigen Seite
zurück. Die aktive Anwendung wird unter sway, Hyprland, X11, Windows und
macOS erkannt, GNOME und KDE unter Wayland geben sie nicht preis.

### Display-Automatik

```yaml
display:
  dim_after_s: 300         # nach 5 min ohne Tastendruck dimmen …
  dim_brightness: 10       # … auf 10 %
  off_when_locked: true    # aus, solange der Bildschirm gesperrt ist
  night: {from: "22:00", to: "07:00", brightness: 20}
```

### Home Assistant und OBS

Zugangsdaten stehen nicht in der `deck.yaml`, sondern in einer eigenen
`secrets.yaml` daneben (Rechte `0600`), die der Editor unter
*Einstellungen → Integrationen* pflegt und testet:

```yaml
home_assistant: {url: "http://homeassistant.local:8123", token: "<Long-Lived Access Token>"}
obs: {url: "ws://127.0.0.1:4455", password: "<obs-websocket-Passwort>"}
```

```yaml
- index: 10                # Licht schalten, Taste leuchtet, solange es an ist
  label: Küche
  action: {type: home_assistant, service: light.toggle, entity_id: light.kitchen}
  live: {type: home_assistant, entity_id: light.kitchen}
- index: 11                # Aufnahme starten/stoppen und Status anzeigen
  action: {type: obs, op: toggle_record}
  live: {type: obs, show: record}           # scene | record | stream
- index: 12
  action: {type: obs, op: switch_scene, target: Kamera}
```

OBS-Befehle: `switch_scene`, `toggle_record`, `start_record`, `stop_record`,
`toggle_stream`, `start_stream`, `stop_stream`, `toggle_mute` (mit `target`
als Quelle), `toggle_replay_buffer`, `save_replay`. OBS braucht
obs-websocket 5 (ab OBS 28 eingebaut). Home-Assistant-Widgets zeigen den
Zustand oder ein Attribut (`attribute`) und färben die Taste bei
`active_state` (Standard `on`).

### Sprache

Editor und Gerätetexte folgen der Systemsprache (Deutsch, sonst Englisch).
Im Editor lässt sich die Sprache umschalten; `language: de` bzw.
`language: en` in der `deck.yaml` legt die Texte auf dem Gerät fest.

### Widgets

Statt der Firmware-Anzeige kann die App das Infofenster selbst rendern und
mehrere Widgets im Wechsel zeigen; jede Taste kann ein Widget als
Live-Anzeige tragen.

```yaml
small_window:
  enabled: true
  rotate_every_s: 8
  widgets:
    - {type: clock, format: '%H:%M'}
    - {type: command, title: Wetter, cmd: "curl -s 'wttr.in/Berlin?format=3'", interval_s: 600}
    - {type: graph, metric: cpu}            # cpu | memory | network
    - {type: image, paths: [~/bilder/a.png, ~/bilder/b.png], interval_s: 5}
    - {type: timer, title: Pomodoro}
    - {type: media}                         # Titel, Interpret, Cover
pages:
  main:
    buttons:
      - index: 0                            # rot, solange das Mikrofon stumm ist
        live:
          type: command
          cmd: "pactl get-source-mute @DEFAULT_SOURCE@ | grep -q no && echo MIC"
          ok_color: '#14532D'
          fail_color: '#B91C1C'
      - index: 1                            # Timer starten/pausieren und anzeigen
        action: {type: timer, op: toggle, minutes: 25}   # toggle | start | pause | reset
        live: {type: timer}
```

Befehle laufen mit der Shell des Systems (`sh` bzw. `cmd`), höchstens 10 s,
die ersten vier Zeilen der Ausgabe werden angezeigt. „Läuft gerade“ nutzt
unter Linux MPRIS (Spotify, Firefox, VLC, …), unter Windows die System Media
Transport Controls (alles, was im Lautstärke-Overlay erscheint) und unter
macOS Spotify bzw. die Musik-App.

## Plattformhinweise

**Linux:** Der Zugriff auf `/dev/hidraw*` braucht eine udev-Regel (die
Pakete installieren sie mit):

```bash
sudo cp packaging/linux/70-ulanzi-deck.rules /etc/udev/rules.d/
sudo udevadm control --reload-rules && sudo udevadm trigger
```

Autostart nutzt `~/.config/autostart` (GNOME, KDE, XFCE …). Unter sway &
Co. stattdessen `exec ulanzi-deck` in die Compositor-Config eintragen.

**Windows:** Der Installer arbeitet ohne Adminrechte im Benutzerprofil.
Die offizielle Ulanzi-Studio-Software darf nicht gleichzeitig
laufen.

**macOS:** Die App läuft ohne Dock-Symbol nur im Tray. Für Tastenkürzel und
die Erkennung der aktiven App fragt macOS nach den Freigaben *Bedienungshilfen*
bzw. *Automation*. Die Release-Binaries haben ein Tray-Icon. Mit `scripts/build.sh`
cross-kompilierte Builds haben keins (dafür wäre cgo nötig); auf einem Mac
gebaut (`CGO_ENABLED=1 go build ./cmd/ulanzi-deck`) ist es dabei.

## Entwicklung

Alles läuft im Docker-Container, auf dem Host wird nichts installiert.

```bash
docker compose run --rm dev go test ./...           # Tests
docker compose run --rm dev sh scripts/build.sh     # Binaries für alle Plattformen nach dist/
docker compose run --rm dev sh scripts/fetch-assets.sh   # eingebettete Fonts/Icons neu laden
```

GitHub Actions prüft jeden Push (gofmt, vet, Tests auf Linux, Windows und
macOS, Cross-Build). Ein Tag `v*` baut die Release-Binaries – macOS dabei
nativ mit cgo, damit das Tray-Icon enthalten ist – und legt ein GitHub-Release
an – mit `.deb`/`.rpm`, Windows-Installer und macOS-`.dmg`:

```bash
git tag v0.4.0 && git push origin v0.4.0
sh packaging/aur/update.sh 0.4.0     # danach PKGBUILD für das AUR, siehe packaging/aur/README.md
```

Aufbau (Domain-Driven):

```text
cmd/ulanzi-deck/              CLI, Start, Verdrahtung
internal/domain/deck/         Konfiguration, Tasten, Aktionen, Regeln
internal/application/         Daemon (Seiten, Infofenster, Aktionen, Automatik), Widgets
internal/infrastructure/
  d200/                       HID-Protokoll, ZIP-Upload, Gerätetreiber mit Reconnect
  configfile/                 deck.yaml lesen/schreiben
  render/                     Tasten- und Infofenster-Grafiken, eingebettete Fonts
  catalog/                    Icon-Katalog (Font Awesome, Twemoji)
  actions/                    Befehle, Kürzel, URLs je Betriebssystem
  metrics/                    Systemwerte
  media/                      „Läuft gerade“ (MPRIS, SMTC, AppleScript)
  desktop/                    aktive Anwendung, Bildschirmsperre
  integrations/               Home Assistant, OBS, secrets.yaml
  autostart/                  Autostart je Betriebssystem
packaging/                    deb/rpm (nfpm), AUR, Windows-Installer, macOS-App
internal/interfaces/
  web/                        HTTP-API und Editor (Alpine.js)
  tray/                       Tray-Icon
```

## Danksagung und Vorarbeiten

Ohne das Reverse Engineering der Community gäbe es dieses Projekt nicht:

- **[marcelobrake/ulanzi-linux](https://github.com/marcelobrake/ulanzi-linux)**
  – inoffizieller Linux-Client in Python. Protokoll, Gerätetreiber,
  ZIP-Aufbau samt Firmware-Workarounds und Daemon-Logik sind eine
  Go-Portierung davon; das `deck.yaml`-Format ist kompatibel.
- **[redphx/strmdck](https://github.com/redphx/strmdck)** – die ursprüngliche
  Python-Bibliothek, die das HID-Protokoll des D200 entschlüsselt hat.
- **[redphx/homedeck](https://github.com/redphx/homedeck)** –
  Home-Assistant-Integration auf Basis von strmdck, gute Referenz für das
  Rendern von Icons.
- **[UlanziTechnology/UlanziDeckPlugin-SDK](https://github.com/UlanziTechnology/UlanziDeckPlugin-SDK)**
  – das offizielle Plugin-SDK der Windows-/Mac-Software, nützlich zum
  Abgleich von Manifest und Icon-Größen.
- **[Hackaday](https://hackaday.com/tag/ulanzi-d200/)** – Berichte, die
  zeigten, dass auf dem Gerät Linux 5.10 auf einem Rockchip RK3308HS mit
  offenem ADB-Root läuft.
- **[rafaelmartins/usbhid](https://rafaelmartins.com/p/usbhid/)** – HID in
  purem Go für alle drei Betriebssysteme.

## Entstehung

Dieses Projekt ist KI-generiert: Code, Tests, Oberfläche und Dokumentation
wurden größtenteils von einem KI-Assistenten geschrieben – als
Portierung von ulanzi-linux nach Go,
erweitert um Plattformunterstützung für Windows und macOS, einen neuen
Editor und weitere Aktionen. Anforderungen, Entscheidungen, Review und die
Tests am echten D200 kamen vom Maintainer.

Getestet ist die App am echten Gerät unter Linux (Wayland/sway). Die
Windows- und macOS-Builds kompilieren und sind durch Unit-Tests abgedeckt,
wurden aber noch nicht auf echter Hardware ausprobiert – Rückmeldungen und
Pull Requests sind willkommen.

## Lizenz

MIT – siehe [LICENSE](LICENSE). Protokoll und Treiber-Logik sind eine
Go-Portierung von [ulanzi-linux](https://github.com/marcelobrake/ulanzi-linux)
(MIT); dessen Vermerk sowie die Lizenzen der eingebetteten Fonts, Icons und
von Alpine.js stehen in [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).

## Hinweis

Inoffizielles Projekt, nicht mit Ulanzi oder Fuzhou Rockchip Electronics
verbunden. „Ulanzi“ und „Stream Controller D200“ sind Marken der jeweiligen
Inhaber. Nutzung auf eigenes Risiko.
