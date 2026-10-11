package configfile

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBackupsArePrunedAndListedNewestFirst(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deck.yaml")
	start := time.Date(2026, 10, 11, 12, 0, 0, 0, time.Local)
	for i := range MaxBackups + 5 {
		if _, err := WriteBackup(path, []byte(fmt.Sprint(i)), start.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	backups, err := ListBackups(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != MaxBackups {
		t.Fatalf("kept %d backups", len(backups))
	}
	newest, _ := ReadBackup(path, backups[0].Name)
	if string(newest) != fmt.Sprint(MaxBackups+4) || !backups[0].Saved.Equal(start.Add(time.Duration(MaxBackups+4)*time.Second)) {
		t.Fatalf("newest %q at %v", newest, backups[0].Saved)
	}
}

func TestSameSecondBackupsKeepOrder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deck.yaml")
	now := time.Now()
	for _, text := range []string{"a", "b", "c"} {
		if _, err := WriteBackup(path, []byte(text), now); err != nil {
			t.Fatal(err)
		}
	}
	backups, _ := ListBackups(path)
	newest, _ := ReadBackup(path, backups[0].Name)
	if len(backups) != 3 || string(newest) != "c" {
		t.Fatalf("got %d backups, newest %q", len(backups), newest)
	}
}

func TestReadBackupRejectsForeignNames(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deck.yaml")
	for _, name := range []string{"../deck.yaml", "secrets.yaml", "deck-20261011-120000.yaml/../../x"} {
		if _, err := ReadBackup(path, name); err != ErrBackupNotFound {
			t.Fatalf("%q: %v", name, err)
		}
	}
}

func TestMigrateSnapshots(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "deck.yaml")
	for _, name := range []string{"deck.yaml", "deck-20261004-230440.yaml", "deck-20261004-230440-01.yaml", "deck-firmware-20261004-230440.zip", "other.yaml"} {
		os.WriteFile(filepath.Join(dir, name), []byte(name), 0o644)
	}
	moved, err := MigrateSnapshots(path)
	if err != nil || moved != 2 {
		t.Fatalf("moved %d: %v", moved, err)
	}
	backups, _ := ListBackups(path)
	if len(backups) != 2 {
		t.Fatalf("backups %v", backups)
	}
	if _, err := os.Stat(filepath.Join(dir, "other.yaml")); err != nil {
		t.Fatal("unrelated file touched")
	}
}
