package actions

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/maffmeier/ulanzi-deck-configurator/internal/domain/deck"
)

// PredefinedInfo describes a built-in action for the editor's action list.
type PredefinedInfo struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Group string `json:"group"`
	Icon  string `json:"icon"`
}

type predefined struct {
	id      string
	label   string
	group   string
	icon    string
	aliases []string
	// action returns nil when the command has no implementation on goos.
	action func(goos string) *deck.Action
}

const (
	groupMedia   = "Medien & Audio"
	groupSystem  = "System"
	groupEdit    = "Bearbeiten"
	groupBrowser = "Browser & Fenster"
	groupMeeting = "Videokonferenz"
)

func keys(spec string) func(string) *deck.Action {
	return func(string) *deck.Action { return &deck.Action{Type: deck.ActionShortcut, Keys: spec} }
}

// modKeys maps "mod" to cmd on macOS and ctrl elsewhere, the usual split
// for editing and browser shortcuts.
func modKeys(spec string) func(string) *deck.Action {
	return func(goos string) *deck.Action {
		mod := "ctrl"
		if goos == "darwin" {
			mod = "cmd"
		}
		return &deck.Action{Type: deck.ActionShortcut, Keys: strings.ReplaceAll(spec, "mod", mod)}
	}
}

// perOS picks an action per platform; an empty entry means unsupported.
type perOS struct{ linux, windows, darwin *deck.Action }

func (p perOS) pick(goos string) *deck.Action {
	switch goos {
	case "windows":
		return p.windows
	case "darwin":
		return p.darwin
	}
	return p.linux
}

func sh(cmd string) *deck.Action            { return &deck.Action{Type: deck.ActionShell, Cmd: cmd} }
func key(k string) *deck.Action             { return &deck.Action{Type: deck.ActionShortcut, Keys: k} }
func os3(p perOS) func(string) *deck.Action { return p.pick }

var predefinedCommands = []predefined{
	// Medien & Audio
	{id: "media_play_pause", label: "Wiedergabe / Pause", group: groupMedia, icon: "fa:solid:play", action: keys("XF86AudioPlay")},
	{id: "media_next", aliases: []string{"media_next_track"}, label: "Nächster Titel", group: groupMedia, icon: "fa:solid:forward-step", action: keys("XF86AudioNext")},
	{id: "media_previous", aliases: []string{"media_prev", "media_previous_track"}, label: "Vorheriger Titel", group: groupMedia, icon: "fa:solid:backward-step", action: keys("XF86AudioPrev")},
	{id: "media_stop", label: "Stopp", group: groupMedia, icon: "fa:solid:stop", action: os3(perOS{key("XF86AudioStop"), key("XF86AudioStop"), sh(`osascript -e 'tell application "Music" to stop'`)})},
	{id: "audio_volume_up", aliases: []string{"volume_up"}, label: "Lauter", group: groupMedia, icon: "fa:solid:volume-high", action: keys("XF86AudioRaiseVolume")},
	{id: "audio_volume_down", aliases: []string{"volume_down"}, label: "Leiser", group: groupMedia, icon: "fa:solid:volume-low", action: keys("XF86AudioLowerVolume")},
	{id: "audio_mute", aliases: []string{"volume_mute"}, label: "Ton aus", group: groupMedia, icon: "fa:solid:volume-xmark", action: keys("XF86AudioMute")},
	{id: "audio_mic_mute", label: "Mikrofon aus", group: groupMedia, icon: "fa:solid:microphone-slash", action: os3(perOS{key("XF86AudioMicMute"), nil, key("XF86AudioMicMute")})},

	// System
	{id: "system_lock_screen", label: "Bildschirm sperren", group: groupSystem, icon: "fa:solid:lock", action: os3(perOS{
		sh("loginctl lock-session"),
		sh("rundll32.exe user32.dll,LockWorkStation"),
		key("ctrl+cmd+q"),
	})},
	{id: "system_suspend", label: "Ruhezustand", group: groupSystem, icon: "fa:solid:moon", action: os3(perOS{
		sh("systemctl suspend"),
		sh("rundll32.exe powrprof.dll,SetSuspendState 0,1,0"),
		sh("pmset sleepnow"),
	})},
	{id: "gnome_terminal", aliases: []string{"system_terminal"}, label: "Terminal öffnen", group: groupSystem, icon: "fa:solid:terminal", action: os3(perOS{
		// $TERMINAL is the de-facto standard on tiling setups.
		sh(`${TERMINAL:-$(command -v x-terminal-emulator || command -v gnome-terminal || command -v konsole || command -v alacritty || command -v foot || command -v kitty || echo xterm)}`),
		sh("start wt || start cmd"),
		sh("open -a Terminal"),
	})},
	{id: "system_file_manager", label: "Dateimanager", group: groupSystem, icon: "fa:solid:folder-open", action: os3(perOS{
		sh(`xdg-open "$HOME"`), sh("start explorer"), sh(`open "$HOME"`),
	})},
	{id: "system_downloads", label: "Downloads öffnen", group: groupSystem, icon: "fa:solid:download", action: os3(perOS{
		sh(`xdg-open "$(xdg-user-dir DOWNLOAD 2>/dev/null || echo "$HOME/Downloads")"`),
		sh(`start "" "%USERPROFILE%\Downloads"`),
		sh(`open "$HOME/Downloads"`),
	})},
	{id: "system_calculator", label: "Taschenrechner", group: groupSystem, icon: "fa:solid:calculator", action: os3(perOS{
		sh("gnome-calculator || kcalc || qalculate-gtk || galculator"),
		sh("start calc"),
		sh("open -a Calculator"),
	})},
	{id: "display_screenshot", label: "Bildschirmfoto", group: groupSystem, icon: "fa:solid:display", action: os3(perOS{
		key("Print"), key("super+Print"), key("cmd+shift+3"),
	})},
	{id: "display_screenshot_selection", aliases: []string{"gnome_screenshot"}, label: "Bildschirmausschnitt", group: groupSystem, icon: "fa:solid:crop-simple", action: os3(perOS{
		// GNOME, KDE, then wlroots compositors (sway, Hyprland): selection to
		// the clipboard and ~/Pictures.
		sh(`if command -v gnome-screenshot >/dev/null; then gnome-screenshot -i; ` +
			`elif command -v spectacle >/dev/null; then spectacle -r; ` +
			`else f="$(xdg-user-dir PICTURES 2>/dev/null || echo "$HOME")/screenshot-$(date +%Y%m%d-%H%M%S).png"; ` +
			`grim -g "$(slurp)" "$f" && wl-copy < "$f"; fi`),
		key("super+shift+s"), key("cmd+shift+4"),
	})},
	{id: "gnome_show_applications", label: "Anwendungen (GNOME)", group: groupSystem, icon: "fa:solid:table-cells", action: os3(perOS{key("super+a"), nil, nil})},

	// Bearbeiten
	{id: "edit_copy", label: "Kopieren", group: groupEdit, icon: "fa:solid:copy", action: modKeys("mod+c")},
	{id: "edit_cut", label: "Ausschneiden", group: groupEdit, icon: "fa:solid:scissors", action: modKeys("mod+x")},
	{id: "edit_paste", label: "Einfügen", group: groupEdit, icon: "fa:solid:paste", action: modKeys("mod+v")},
	{id: "edit_undo", label: "Rückgängig", group: groupEdit, icon: "fa:solid:rotate-left", action: modKeys("mod+z")},
	{id: "edit_redo", label: "Wiederholen", group: groupEdit, icon: "fa:solid:rotate-right", action: os3(perOS{key("ctrl+shift+z"), key("ctrl+y"), key("cmd+shift+z")})},
	{id: "edit_select_all", label: "Alles auswählen", group: groupEdit, icon: "fa:solid:object-group", action: modKeys("mod+a")},
	{id: "edit_save", label: "Speichern", group: groupEdit, icon: "fa:solid:floppy-disk", action: modKeys("mod+s")},
	{id: "edit_find", label: "Suchen", group: groupEdit, icon: "fa:solid:magnifying-glass", action: modKeys("mod+f")},

	// Browser & Fenster
	{id: "browser_new_tab", label: "Neuer Tab", group: groupBrowser, icon: "fa:solid:square-plus", action: modKeys("mod+t")},
	{id: "browser_close_tab", label: "Tab schließen", group: groupBrowser, icon: "fa:solid:xmark", action: modKeys("mod+w")},
	{id: "browser_reopen_tab", label: "Tab wiederherstellen", group: groupBrowser, icon: "fa:solid:clock-rotate-left", action: modKeys("mod+shift+t")},
	{id: "browser_next_tab", label: "Nächster Tab", group: groupBrowser, icon: "fa:solid:angles-right", action: keys("ctrl+Tab")},
	{id: "browser_prev_tab", label: "Vorheriger Tab", group: groupBrowser, icon: "fa:solid:angles-left", action: keys("ctrl+shift+Tab")},
	{id: "browser_reload", label: "Neu laden", group: groupBrowser, icon: "fa:solid:arrows-rotate", action: os3(perOS{key("F5"), key("F5"), key("cmd+r")})},
	{id: "window_fullscreen", label: "Vollbild", group: groupBrowser, icon: "fa:solid:expand", action: os3(perOS{key("F11"), key("F11"), key("ctrl+cmd+f")})},
	{id: "window_close", label: "Fenster schließen", group: groupBrowser, icon: "fa:solid:rectangle-xmark", action: os3(perOS{key("alt+F4"), key("alt+F4"), key("cmd+w")})},

	// Videokonferenz – the default shortcuts of the desktop/web apps; they
	// only work while the meeting window has focus.
	{id: "meeting_zoom_mic", label: "Zoom: Mikrofon", group: groupMeeting, icon: "fa:solid:microphone-slash", action: os3(perOS{key("alt+a"), key("alt+a"), key("cmd+shift+a")})},
	{id: "meeting_zoom_camera", label: "Zoom: Kamera", group: groupMeeting, icon: "fa:solid:video-slash", action: os3(perOS{key("alt+v"), key("alt+v"), key("cmd+shift+v")})},
	{id: "meeting_teams_mic", label: "Teams: Mikrofon", group: groupMeeting, icon: "fa:brands:microsoft", action: modKeys("mod+shift+m")},
	{id: "meeting_teams_camera", label: "Teams: Kamera", group: groupMeeting, icon: "fa:solid:video-slash", action: modKeys("mod+shift+o")},
	{id: "meeting_meet_mic", label: "Meet: Mikrofon", group: groupMeeting, icon: "fa:brands:google", action: modKeys("mod+d")},
	{id: "meeting_meet_camera", label: "Meet: Kamera", group: groupMeeting, icon: "fa:solid:video-slash", action: modKeys("mod+e")},
}

// Predefined lists the commands available on this platform.
func Predefined() []PredefinedInfo {
	var list []PredefinedInfo
	for _, p := range predefinedCommands {
		if p.action(runtime.GOOS) == nil {
			continue
		}
		list = append(list, PredefinedInfo{ID: p.id, Label: p.label, Group: p.group, Icon: p.icon})
	}
	return list
}

func ResolvePredefined(id string) (deck.Action, error) {
	return resolvePredefined(id, runtime.GOOS)
}

func resolvePredefined(id, goos string) (deck.Action, error) {
	for _, p := range predefinedCommands {
		if p.id != id && !contains(p.aliases, id) {
			continue
		}
		action := p.action(goos)
		if action == nil {
			return deck.Action{}, fmt.Errorf("%q is not available on %s", id, goos)
		}
		return *action, nil
	}
	return deck.Action{}, fmt.Errorf("unknown predefined command id: %q", id)
}

func contains(items []string, s string) bool {
	for _, i := range items {
		if i == s {
			return true
		}
	}
	return false
}
