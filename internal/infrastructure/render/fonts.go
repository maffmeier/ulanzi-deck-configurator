// Package render produces the PNG tiles the D200 displays.
package render

import (
	"embed"
	"fmt"
	"io/fs"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
)

//go:embed fonts/*.ttf
var fontFiles embed.FS

type fontVariant struct{ bold, italic bool }

var familyFiles = map[string]map[fontVariant]string{
	"DejaVu Sans": {
		{false, false}: "DejaVuSans.ttf",
		{true, false}:  "DejaVuSans-Bold.ttf",
		{false, true}:  "DejaVuSans-Oblique.ttf",
		{true, true}:   "DejaVuSans-BoldOblique.ttf",
	},
	"DejaVu Serif": {
		{false, false}: "DejaVuSerif.ttf",
		{true, false}:  "DejaVuSerif-Bold.ttf",
		{false, true}:  "DejaVuSerif-Italic.ttf",
		{true, true}:   "DejaVuSerif-BoldItalic.ttf",
	},
	"DejaVu Sans Mono": {
		{false, false}: "DejaVuSansMono.ttf",
		{true, false}:  "DejaVuSansMono-Bold.ttf",
		{false, true}:  "DejaVuSansMono-Oblique.ttf",
		{true, true}:   "DejaVuSansMono-BoldOblique.ttf",
	},
	"Liberation Sans": {
		{false, false}: "LiberationSans-Regular.ttf",
		{true, false}:  "LiberationSans-Bold.ttf",
		{false, true}:  "LiberationSans-Italic.ttf",
		{true, true}:   "LiberationSans-BoldItalic.ttf",
	},
	"Liberation Serif": {
		{false, false}: "LiberationSerif-Regular.ttf",
		{true, false}:  "LiberationSerif-Bold.ttf",
		{false, true}:  "LiberationSerif-Italic.ttf",
		{true, true}:   "LiberationSerif-BoldItalic.ttf",
	},
}

var (
	parsedMu sync.Mutex
	parsed   = map[string]*opentype.Font{}
	facesMu  sync.Mutex
	faces    = map[string]font.Face{}
)

func loadFont(file string) (*opentype.Font, error) {
	parsedMu.Lock()
	defer parsedMu.Unlock()
	if f, ok := parsed[file]; ok {
		return f, nil
	}
	data, err := fontFiles.ReadFile("fonts/" + file)
	if err != nil {
		return nil, err
	}
	f, err := opentype.Parse(data)
	if err != nil {
		return nil, err
	}
	parsed[file] = f
	return f, nil
}

// Face returns a cached face; unknown families fall back to DejaVu Sans.
func Face(family string, bold, italic bool, size float64) font.Face {
	files, ok := familyFiles[family]
	if !ok {
		files = familyFiles["DejaVu Sans"]
	}
	file := files[fontVariant{bold, italic}]
	key := fmt.Sprintf("%s@%g", file, size)

	facesMu.Lock()
	defer facesMu.Unlock()
	if face, ok := faces[key]; ok {
		return face
	}
	f, err := loadFont(file)
	if err != nil {
		panic(fmt.Sprintf("embedded font %s: %v", file, err))
	}
	face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		panic(fmt.Sprintf("embedded font %s: %v", file, err))
	}
	faces[key] = face
	return face
}

// FontFS exposes the embedded TTFs so the editor preview uses the exact
// fonts the deck renders with.
func FontFS() fs.FS {
	sub, _ := fs.Sub(fontFiles, "fonts")
	return sub
}
