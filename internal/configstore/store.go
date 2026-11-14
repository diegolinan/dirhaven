// Package configstore persists only explicitly approved filesystem scopes.
// It never stores credentials, changes MCP host settings, or starts processes.
package configstore

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	SchemaVersion = 1
	MaxBytes      = 16 << 10
)

// Scope is an opt-in configuration snapshot, not a running MCP connection.
type Scope struct {
	Version int      `json:"version"`
	Mode    string   `json:"mode"`
	Roots   []string `json:"roots,omitempty"`
}

// Store uses a per-user OS config directory by default. WithPath is intended
// for isolated tests, never an alternate global configuration store.
type Store struct{ Path string }

func Default() (Store, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return Store{}, fmt.Errorf("locate per-user config directory: %w", err)
	}
	return Store{Path: filepath.Join(base, "dirhaven", "scope.json")}, nil
}

func WithPath(path string) Store { return Store{Path: path} }

func (s Store) Load() (Scope, string, bool, error) {
	var empty Scope
	if s.Path == "" || !filepath.IsAbs(s.Path) {
		return empty, "", false, errors.New("config path must be absolute")
	}
	info, err := os.Lstat(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return empty, "", false, nil
	}
	if err != nil {
		return empty, "", false, fmt.Errorf("inspect saved config: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() > MaxBytes {
		return empty, "", false, errors.New("saved config must be a regular small file")
	}
	f, err := os.Open(s.Path)
	if err != nil {
		return empty, "", false, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, MaxBytes+1))
	if err != nil || len(data) > MaxBytes {
		return empty, "", false, errors.New("saved config exceeds maximum size")
	}
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	var value Scope
	if err := dec.Decode(&value); err != nil {
		return empty, "", false, fmt.Errorf("invalid saved config: %w", err)
	}
	var trailing any
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		return empty, "", false, errors.New("unexpected content in saved config")
	}
	if err := Validate(value); err != nil {
		return empty, "", false, err
	}
	return value, revision(data), true, nil
}

func Validate(value Scope) error {
	if value.Version != SchemaVersion {
		return fmt.Errorf("unsupported saved config version %d", value.Version)
	}
	switch value.Mode {
	case "scoped":
		if len(value.Roots) == 0 || len(value.Roots) > 24 {
			return errors.New("saved scope must contain between 1 and 24 directories")
		}
		seen := map[string]bool{}
		for _, path := range value.Roots {
			if strings.TrimSpace(path) != path || !filepath.IsAbs(path) || filepath.Clean(path) != path {
				return errors.New("saved config contains an invalid absolute directory")
			}
			key := path
			if runtime.GOOS == "windows" {
				key = strings.ToLower(key)
			}
			if seen[key] {
				return errors.New("saved config contains duplicate directories")
			}
			seen[key] = true
		}
	case "all":
		if len(value.Roots) != 0 {
			return errors.New("broad scope must not contain explicit roots")
		}
	default:
		return errors.New("saved config has invalid access mode")
	}
	return nil
}

// Save replaces a snapshot only when the last observed revision matches the
// file on disk. The caller must validate real directories and broad opt-in
// separately before calling Save.
func (s Store) Save(value Scope, expectedRevision string) (string, error) {
	if err := Validate(value); err != nil {
		return "", err
	}
	_, current, exists, err := s.Load()
	if err != nil {
		return "", err
	}
	if expectedRevision != current || (!exists && expectedRevision != "") {
		return "", errors.New("saved configuration changed; reload before saving")
	}
	parent := filepath.Dir(s.Path)
	if err := os.MkdirAll(parent, 0700); err != nil {
		return "", fmt.Errorf("create per-user config directory: %w", err)
	}
	if info, err := os.Lstat(parent); err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("config directory is not a regular directory")
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "", err
	}
	data = append(data, '\n')
	if len(data) > MaxBytes {
		return "", errors.New("saved config exceeds maximum size")
	}
	tmp, err := os.CreateTemp(parent, ".scope-*.tmp")
	if err != nil {
		return "", err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return "", err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	// Check again just before replacement to avoid overwriting a known change.
	_, latest, _, err := s.Load()
	if err != nil || latest != expectedRevision {
		return "", errors.New("saved configuration changed during write; reload")
	}
	if err := os.Rename(name, s.Path); err != nil {
		return "", fmt.Errorf("replace saved config atomically: %w", err)
	}
	return revision(data), nil
}

// Remove is an explicit revoke action. Empty expectedRevision cannot delete a
// saved file, so a stale browser tab cannot silently erase a newer decision.
func (s Store) Remove(expectedRevision string) error {
	_, current, exists, err := s.Load()
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	if expectedRevision == "" || current != expectedRevision {
		return errors.New("saved configuration changed; reload before removal")
	}
	return os.Remove(s.Path)
}

func revision(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
