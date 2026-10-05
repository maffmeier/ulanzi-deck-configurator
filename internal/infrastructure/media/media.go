// Package media reports what the desktop is currently playing.
package media

import (
	"errors"
	"image"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/maffmeier/ulanzi-deck-configurator/internal/infrastructure/render"
)

var ErrUnsupported = errors.New("media info is not supported on this platform")

type Info struct {
	Title   string
	Artist  string
	Playing bool
	ArtURL  string
}

// Current returns the active player, preferring one that is playing.
func Current() (Info, error) { return current() }

var (
	coverMu    sync.Mutex
	coverURL   string
	coverImage image.Image
	httpClient = &http.Client{Timeout: 5 * time.Second}
)

// Cover loads file:// and http(s) artwork; the last one is cached because
// the widget asks for it every refresh.
func Cover(raw string) image.Image {
	if raw == "" {
		return nil
	}
	coverMu.Lock()
	defer coverMu.Unlock()
	if raw == coverURL {
		return coverImage
	}
	coverURL, coverImage = raw, loadCover(raw)
	return coverImage
}

func loadCover(raw string) image.Image {
	if img, ok := platformCover(raw); ok {
		return img
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil
	}
	switch u.Scheme {
	case "file":
		f, err := os.Open(u.Path)
		if err != nil {
			return nil
		}
		defer f.Close()
		img, _ := render.DecodeImage(f)
		return img
	case "http", "https":
		resp, err := httpClient.Get(raw)
		if err != nil {
			return nil
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil
		}
		img, _ := render.DecodeImage(resp.Body)
		return img
	}
	return nil
}
