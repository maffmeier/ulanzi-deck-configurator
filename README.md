# Ulanzi Deck

Steuerung und Konfiguration für den **Ulanzi Stream Controller D200**
(USB `2207:0019`) unter Linux, Windows und macOS – eine einzelne Binary ohne
Abhängigkeiten.

- Tasten mit Icons oder Text, beliebig viele Seiten, feste Tasten auf allen Seiten
- Aktionen: Befehl ausführen, Tastenkürzel, Website öffnen, Seite wechseln
  (inkl. „nächste“/„vorherige“ Seite), Medien- und Lautstärketasten
- Infofenster: Uhr, CPU/RAM der Firmware oder eigene Messwerte
  (CPU, RAM, GPU, Temperatur, Festplatte, Netzwerk, Akku)
- Web-Editor im Stil der Stream-Deck-Software mit Drag & Drop und
  eingebautem Icon-Katalog (Font Awesome Free + Twemoji)
- Tray-Icon, Autostart, automatische Wiederverbindung nach dem Abstecken
- Änderungen an der `deck.yaml` werden ohne Neustart übernommen

Das Protokoll stammt aus dem Reverse Engineering von
[ulanzi-linux](https://github.com/marcelobrake/ulanzi-linux) und
[strmdck](https://github.com/redphx/strmdck); das YAML-Format ist zu
ulanzi-linux kompatibel.

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
| Linux   | Wayland: `wtype`, X11: `xdotool` (muss installiert sein) |
| Windows | `SendInput`, keine Abhängigkeiten |
| macOS   | `osascript`; ulanzi-deck braucht die Freigabe unter *Datenschutz & Sicherheit → Bedienungshilfen* |

Unter tiling Compositors (sway, Hyprland) ist oft ein Befehl wie
`swaymsg exec alacritty` robuster als ein simuliertes Kürzel.

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

**macOS:** Cross-kompilierte Builds haben kein Tray-Icon (dafür wäre cgo
nötig). Auf einem Mac gebaut (`CGO_ENABLED=1 go build ./cmd/ulanzi-deck`)
ist es dabei.

## Entwicklung

Alles läuft im Docker-Container, auf dem Host wird nichts installiert.

```bash
docker compose run --rm dev go test ./...           # Tests
docker compose run --rm dev sh scripts/build.sh     # Binaries für alle Plattformen nach dist/
docker compose run --rm dev sh scripts/fetch-assets.sh   # eingebettete Fonts/Icons neu laden
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

## Lizenzen der eingebetteten Assets

DejaVu Fonts (Bitstream Vera License), Liberation Fonts (SIL OFL 1.1),
Font Awesome Free (Icons CC BY 4.0, Fonts SIL OFL 1.1),
Twemoji (CC BY 4.0), Alpine.js (MIT). Die Lizenztexte der Fonts und Icons liegen neben den
Dateien unter `internal/infrastructure/*/`.
