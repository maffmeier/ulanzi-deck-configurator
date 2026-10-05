package media

import (
	"image"
	"strings"
	"sync"

	"github.com/godbus/dbus/v5"
)

const mprisPrefix = "org.mpris.MediaPlayer2."

var (
	busOnce sync.Once
	bus     *dbus.Conn
	busErr  error
)

// current asks every MPRIS player on the session bus (Spotify, Firefox,
// VLC, mpv with mpris plugin, ...).
func current() (Info, error) {
	busOnce.Do(func() { bus, busErr = dbus.ConnectSessionBus() })
	if busErr != nil {
		return Info{}, busErr
	}
	var names []string
	if err := bus.BusObject().Call("org.freedesktop.DBus.ListNames", 0).Store(&names); err != nil {
		return Info{}, err
	}
	var best Info
	found := false
	for _, name := range names {
		if !strings.HasPrefix(name, mprisPrefix) {
			continue
		}
		info, ok := player(name)
		if !ok {
			continue
		}
		if !found || (info.Playing && !best.Playing) {
			best, found = info, true
		}
	}
	return best, nil
}

func player(name string) (Info, bool) {
	obj := bus.Object(name, "/org/mpris/MediaPlayer2")
	status, err := obj.GetProperty("org.mpris.MediaPlayer2.Player.PlaybackStatus")
	if err != nil {
		return Info{}, false
	}
	meta, err := obj.GetProperty("org.mpris.MediaPlayer2.Player.Metadata")
	if err != nil {
		return Info{}, false
	}
	values, _ := meta.Value().(map[string]dbus.Variant)
	info := Info{Playing: status.Value() == "Playing"}
	if v, ok := values["xesam:title"]; ok {
		info.Title, _ = v.Value().(string)
	}
	if v, ok := values["xesam:artist"]; ok {
		if artists, ok := v.Value().([]string); ok {
			info.Artist = strings.Join(artists, ", ")
		}
	}
	if v, ok := values["mpris:artUrl"]; ok {
		info.ArtURL, _ = v.Value().(string)
	}
	return info, info.Title != ""
}

// platformCover: covers come as file:// or http(s) URLs here.
func platformCover(string) (image.Image, bool) { return nil, false }
