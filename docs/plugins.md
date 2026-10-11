# Plugins

🇬🇧 [English version](plugins.en.md)

Ein Plugin ist ein beliebiges Programm, das dem Deck eigene Aktionen und
Live-Anzeigen hinzufügt. Es läuft als eigener Prozess und spricht über
stdin/stdout zeilenweise JSON – die Sprache ist egal.

## Installation

Jedes Plugin liegt in einem eigenen Ordner unter `plugins/` neben der
`deck.yaml`, z. B. `~/.config/ulanzi-deck/plugins/weather/`. Der Editor
zeigt installierte Plugins in der Aktionsliste unter „Plugins“; nach dem
Kopieren eines Plugins den Editor neu laden.

Ein Beispiel liegt in [examples/plugins/weather](../examples/plugins/weather).

## plugin.yaml

```yaml
name: Wetter                       # Anzeigename
command: [python3, weather.py]     # Programm und Argumente; ./prog läuft aus dem Plugin-Ordner
actions:
  - id: refresh                    # wird als op in der deck.yaml gespeichert
    label: Wetter aktualisieren
    icon: fa:solid:cloud-sun       # optional, Icon aus dem Katalog
    settings:                      # optional: Eingabefelder im Editor
      - {key: city, label: Ort, placeholder: Berlin}
widgets:
  - id: current
    label: Aktuelles Wetter
    settings:
      - {key: city, label: Ort, placeholder: Berlin}
```

In der `deck.yaml`:

```yaml
- index: 3
  action: {type: plugin, plugin: weather, op: refresh}
  live: {type: plugin, plugin: weather, show: current, settings: {city: Kiel}}
```

## Protokoll

Das Deck startet das Programm beim ersten Gebrauch, mit dem Plugin-Ordner
als Arbeitsverzeichnis und `ULANZI_DECK_VERSION` in der Umgebung. Beendet
sich das Programm, wird es beim nächsten Gebrauch (frühestens nach 5 s)
neu gestartet. Jede Nachricht ist ein JSON-Objekt in einer Zeile.

Vom Deck an das Plugin (stdin):

| Nachricht | Bedeutung |
| --------- | --------- |
| `{"event":"start","version":"v0.5.0"}` | direkt nach dem Start |
| `{"event":"action","action":"refresh","settings":{…}}` | eine Taste mit dieser Aktion wurde gedrückt |
| `{"event":"widget","widget":"current","instance":"current-1a2b…","settings":{…}}` | eine Live-Anzeige wird gebraucht; einmal pro Kombination aus Widget und Einstellungen |

Vom Plugin an das Deck (stdout):

| Nachricht | Bedeutung |
| --------- | --------- |
| `{"update":{"instance":"current-1a2b…","title":"Kiel","lines":["14°C","Regen"],"color":"#1E3A8A"}}` | neuer Inhalt einer Live-Anzeige; `color` (Hintergrund) ist optional, höchstens 4 Zeilen |
| `{"log":"Text"}` | landet im Log des Decks |

Das Plugin schickt Updates, wann immer sich etwas ändert – das Deck fragt
nicht nach. Ausgaben auf stderr erscheinen als Warnung im Log.
