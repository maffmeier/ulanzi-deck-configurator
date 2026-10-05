# Ulanzi Deck

[![CI](https://github.com/maffmeier/ulanzi-deck-configurator/actions/workflows/ci.yml/badge.svg)](https://github.com/maffmeier/ulanzi-deck-configurator/actions/workflows/ci.yml)

Steuerung und Konfiguration für den **Ulanzi Stream Controller D200**
(USB `2207:0019`) unter Linux, Windows und macOS – eine einzelne Binary ohne
Abhängigkeiten.

- Tasten mit Icons oder Text, beliebig viele Seiten, feste Tasten auf allen Seiten
- Aktionen: Befehl ausführen, Tastenkürzel, Website öffnen, Seite wechseln
  (inkl. „nächste“/„vorherige“ Seite), Medien- und Lautstärketasten
- Tastenverhalten pro Taste: langer Druck als Zweitaktion, Wiederholen
  solange gehalten, oder Kürzel gedrückt halten (Push-to-Talk)
- Infofenster: Uhr, CPU/RAM der Firmware oder eigene Messwerte
  (CPU, RAM, GPU, Temperatur, Festplatte, Netzwerk, Akku)
- Widgets im Infofenster und als Live-Anzeige auf Tasten: Befehlsausgabe
  (mit Farbe je Exit-Code), Verlaufsgrafik, Bild/Diashow, Timer,
  „läuft gerade“ mit Cover
- Web-Editor im Stil der Stream-Deck-Software mit Drag & Drop und
  eingebautem Icon-Katalog (Font Awesome Free + Twemoji)
- Tray-Icon, Autostart, automatische Wiederverbindung nach dem Abstecken
- Änderungen an der `deck.yaml` werden ohne Neustart übernommen

Das Protokoll stammt aus dem Reverse Engineering der Community (siehe
[Danksagung](#danksagung-und-vorarbeiten)); das YAML-Format ist zu
ulanzi-linux kompatibel.

> **KI-generiert:** Code, Tests und Dokumentation dieses Projekts wurden
> größtenteils mit KI-Unterstützung erstellt, unter menschlicher Anleitung,
> Prüfung und Tests am echten Gerät. Siehe [Entstehung](#entstehung).

## Installation

Fertige Binaries für Linux, Windows und macOS (amd64/arm64) gibt es unter
[Releases](https://github.com/maffmeier/ulanzi-deck-configurator/releases), Prüfsummen in
`SHA256SUMS`. Die Datei herunterladen, ausführbar machen und starten.

macOS: Die Binaries sind nicht signiert. Beim ersten Start entweder im Finder
per Rechtsklick → *Öffnen* bestätigen oder die Quarantäne entfernen:
`xattr -d com.apple.quarantine ulanzi-deck-darwin-arm64`.

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

**Linux:** Der Zugriff auf `/dev/hidraw*` braucht eine udev-Regel:

```bash
sudo cp packaging/linux/70-ulanzi-deck.rules /etc/udev/rules.d/
sudo udevadm control --reload-rules && sudo udevadm trigger
```

Autostart nutzt `~/.config/autostart` (GNOME, KDE, XFCE …). Unter sway &
Co. stattdessen `exec ulanzi-deck` in die Compositor-Config eintragen.

**Windows:** Die offizielle Ulanzi-Studio-Software darf nicht gleichzeitig
laufen.

**macOS:** Die Release-Binaries haben ein Tray-Icon. Mit `scripts/build.sh`
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
an:

```bash
git tag v0.1.0 && git push origin v0.1.0
```

Aufbau (Domain-Driven):

```text
cmd/ulanzi-deck/              CLI, Start, Verdrahtung
internal/domain/deck/         Konfiguration, Tasten, Aktionen, Regeln
internal/application/         Daemon (Seiten, Infofenster, Aktionen), Config-Watcher
internal/infrastructure/
  d200/                       HID-Protokoll, ZIP-Upload, Gerätetreiber mit Reconnect
  configfile/                 deck.yaml lesen/schreiben
  render/                     Tasten- und Infofenster-Grafiken, eingebettete Fonts
  catalog/                    Icon-Katalog (Font Awesome, Twemoji)
  actions/                    Befehle, Kürzel, URLs je Betriebssystem
  metrics/                    Systemwerte
  autostart/                  Autostart je Betriebssystem
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
