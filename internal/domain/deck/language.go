package deck

// SystemLanguage is used when the config doesn't pick a language; the
// program sets it from the OS locale at startup.
var SystemLanguage = "de"

// EffectiveLanguage is the language for texts shown on the deck.
func (c *Config) EffectiveLanguage() string {
	if c.Language == "de" || c.Language == "en" {
		return c.Language
	}
	return SystemLanguage
}

// DeckText returns a short text for the device in the given language.
func DeckText(lang, key string) string {
	if t, ok := deckTexts[lang][key]; ok {
		return t
	}
	return deckTexts["de"][key]
}

var deckTexts = map[string]map[string]string{
	"de": {
		"back": "← Zurück", "error": "Fehler", "imageMissing": "Bild fehlt", "timer": "Timer",
		"done": "Fertig", "nothingPlaying": "Keine Wiedergabe", "offline": "offline", "off": "aus",
		"haOffline": "HA offline", "cpu": "CPU", "memory": "RAM", "network": "Netzwerk",
		"volume": "Lautstärke", "mic": "Mikrofon", "output": "Ausgabe", "muted": "stumm",
		"micOn": "an", "micOff": "stumm",
		"noMeeting": "kein Meeting", "camera": "Kamera", "hand": "Hand", "handUp": "gehoben", "on": "an",
		"calendar": "Termin", "noEvents": "keine Termine", "now": "jetzt", "inMinutes": "in %d min",
		"today": "heute", "tomorrow": "morgen",
		"wd0": "So", "wd1": "Mo", "wd2": "Di", "wd3": "Mi", "wd4": "Do", "wd5": "Fr", "wd6": "Sa",
	},
	"en": {
		"back": "← Back", "error": "Error", "imageMissing": "Image missing", "timer": "Timer",
		"done": "Done", "nothingPlaying": "Nothing playing", "offline": "offline", "off": "off",
		"haOffline": "HA offline", "cpu": "CPU", "memory": "RAM", "network": "Network",
		"volume": "Volume", "mic": "Microphone", "output": "Output", "muted": "muted",
		"micOn": "on", "micOff": "muted",
		"noMeeting": "no meeting", "camera": "Camera", "hand": "Hand", "handUp": "raised", "on": "on",
		"calendar": "Next", "noEvents": "no events", "now": "now", "inMinutes": "in %d min",
		"today": "today", "tomorrow": "tomorrow",
		"wd0": "Sun", "wd1": "Mon", "wd2": "Tue", "wd3": "Wed", "wd4": "Thu", "wd5": "Fri", "wd6": "Sat",
	},
}
