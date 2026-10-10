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
	},
	"en": {
		"back": "← Back", "error": "Error", "imageMissing": "Image missing", "timer": "Timer",
		"done": "Done", "nothingPlaying": "Nothing playing", "offline": "offline", "off": "off",
		"haOffline": "HA offline", "cpu": "CPU", "memory": "RAM", "network": "Network",
	},
}
