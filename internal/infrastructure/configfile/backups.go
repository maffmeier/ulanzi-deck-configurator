package configfile

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

// MaxBackups is how many saved versions are kept per config.
const MaxBackups = 50

// Backup is one saved version of a config.
type Backup struct {
	Name  string
	Saved time.Time
	Size  int64
	order string
}

var ErrBackupNotFound = errors.New("backup not found")

// BackupDir holds the saved versions of the config at path.
func BackupDir(path string) string {
	return filepath.Join(filepath.Dir(path), "backups")
}

// snapshotPattern matches deck-20261004-221500.yaml and deck-20261004-221500-01.yaml.
func snapshotPattern(path string) *regexp.Regexp {
	stem := regexp.QuoteMeta(trimExt(filepath.Base(path)))
	ext := regexp.QuoteMeta(filepath.Ext(path))
	return regexp.MustCompile(`^` + stem + `-(\d{8}-\d{6})(-\d{2})?` + ext + `$`)
}

// WriteBackup stores text as a new version and drops the oldest ones
// beyond MaxBackups.
func WriteBackup(path string, text []byte, now time.Time) (string, error) {
	dir := BackupDir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	target := VersionedPath(filepath.Join(dir, filepath.Base(path)), "", "", now)
	if err := os.WriteFile(target, text, 0o644); err != nil {
		return "", err
	}
	backups, err := ListBackups(path)
	if err != nil {
		return target, nil
	}
	for _, old := range backups[min(len(backups), MaxBackups):] {
		os.Remove(filepath.Join(dir, old.Name))
	}
	return target, nil
}

// ListBackups returns the saved versions, newest first.
func ListBackups(path string) ([]Backup, error) {
	entries, err := os.ReadDir(BackupDir(path))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	pattern := snapshotPattern(path)
	var out []Backup
	for _, e := range entries {
		m := pattern.FindStringSubmatch(e.Name())
		if e.IsDir() || m == nil {
			continue
		}
		saved, err := time.ParseInLocation("20060102-150405", m[1], time.Local)
		if err != nil {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		// Same-second saves get a -NN suffix; the plain name is the first.
		suffix := m[2]
		if suffix == "" {
			suffix = "-00"
		}
		out = append(out, Backup{Name: e.Name(), Saved: saved, Size: info.Size(), order: m[1] + suffix})
	}
	slices.SortFunc(out, func(a, b Backup) int { return strings.Compare(b.order, a.order) })
	return out, nil
}

// ReadBackup returns the content of a version listed by ListBackups.
func ReadBackup(path, name string) ([]byte, error) {
	if !snapshotPattern(path).MatchString(name) {
		return nil, ErrBackupNotFound
	}
	data, err := os.ReadFile(filepath.Join(BackupDir(path), name))
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrBackupNotFound
	}
	return data, err
}

// MigrateSnapshots moves versions that older releases wrote next to the
// config into the backup directory.
func MigrateSnapshots(path string) (int, error) {
	dir := filepath.Dir(path)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	pattern := snapshotPattern(path)
	moved := 0
	for _, e := range entries {
		if e.IsDir() || !pattern.MatchString(e.Name()) {
			continue
		}
		if err := os.MkdirAll(BackupDir(path), 0o755); err != nil {
			return moved, err
		}
		if err := os.Rename(filepath.Join(dir, e.Name()), filepath.Join(BackupDir(path), e.Name())); err != nil {
			return moved, err
		}
		moved++
	}
	return moved, nil
}
