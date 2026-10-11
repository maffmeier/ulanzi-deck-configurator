package configfile

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestProfileRoundTripCarriesImages(t *testing.T) {
	src := t.TempDir()
	os.MkdirAll(filepath.Join(src, "icons"), 0o755)
	os.WriteFile(filepath.Join(src, "icons", "play.png"), []byte("PNG-A"), 0o644)
	os.WriteFile(filepath.Join(src, "icons", "mute.png"), []byte("PNG-B"), 0o644)
	text := `default_page: main
pages:
  main:
    buttons:
      - {index: 0, icon: icons/play.png, action: {type: switch_page, page: tools}}
      - index: 1
        icon: icons/play.png
        toggle: {icon: icons/mute.png}
  tools:
    buttons:
      - {index: 0, label: Werkzeug}
`
	cfg, err := Parse([]byte(text), src)
	if err != nil {
		t.Fatal(err)
	}

	data, err := ExportProfile(cfg, "main")
	if err != nil {
		t.Fatal(err)
	}
	zr, _ := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if len(zr.File) != 3 {
		t.Fatalf("profile files %d, want manifest + 2 images", len(zr.File))
	}

	dst := t.TempDir()
	imported, err := ImportProfile(data, dst)
	if err != nil {
		t.Fatal(err)
	}
	if len(imported.Pages) != 1 || imported.Pages[0].Name != "main" {
		t.Fatalf("pages %+v", imported.Pages)
	}
	b := imported.Pages[0].Buttons
	if content, _ := os.ReadFile(b[0].ResolvedIcon); string(content) != "PNG-A" {
		t.Fatalf("icon %q -> %q", b[0].ResolvedIcon, content)
	}
	if content, _ := os.ReadFile(b[1].Toggle.ResolvedIcon); string(content) != "PNG-B" {
		t.Fatalf("toggle icon %q", content)
	}
	if b[0].ResolvedIcon != b[1].ResolvedIcon {
		t.Fatal("the same image should be packed once")
	}
}

func TestImportProfileIgnoresUnsafeEntries(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range map[string]string{
		"profile.yaml":       "pages:\n  p:\n    buttons: []\n",
		"../evil.png":        "x",
		"icons/../../x.png":  "x",
		"icons/run.sh":       "x",
		"icons/sub/deep.png": "x",
	} {
		w, _ := zw.Create(name)
		w.Write([]byte(content))
	}
	zw.Close()
	parent := t.TempDir()
	dst := filepath.Join(parent, "in")
	if _, err := ImportProfile(buf.Bytes(), dst); err != nil {
		t.Fatal(err)
	}
	var found []string
	filepath.Walk(parent, func(p string, info os.FileInfo, _ error) error {
		if !info.IsDir() {
			found = append(found, p)
		}
		return nil
	})
	if len(found) != 0 {
		t.Fatalf("unsafe entries extracted: %v", found)
	}
}

func TestImportRejectsNonProfiles(t *testing.T) {
	if _, err := ImportProfile([]byte("nope"), t.TempDir()); err == nil {
		t.Fatal("expected error")
	}
}
