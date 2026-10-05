package media

import (
	"image"
	"os/exec"
	"strings"
)

// The script returns "state\ntitle\nartist\nartwork" of Spotify or Music,
// without launching either app.
const nowPlayingScript = `
on info(appName)
	if application appName is not running then return ""
	tell application appName
		if player state is stopped then return ""
		set s to (player state as text)
		set t to name of current track
		set a to artist of current track
		set u to ""
		if appName is "Spotify" then set u to artwork url of current track
		return s & linefeed & t & linefeed & a & linefeed & u
	end tell
end info
set r to info("Spotify")
if r is "" then set r to info("Music")
return r`

func current() (Info, error) {
	out, err := exec.Command("osascript", "-e", nowPlayingScript).Output()
	if err != nil {
		return Info{}, err
	}
	parts := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(parts) < 3 {
		return Info{}, nil
	}
	info := Info{Playing: parts[0] == "playing", Title: parts[1], Artist: parts[2]}
	if len(parts) > 3 {
		info.ArtURL = parts[3]
	}
	return info, nil
}

// platformCover: covers come as file:// or http(s) URLs here.
func platformCover(string) (image.Image, bool) { return nil, false }
