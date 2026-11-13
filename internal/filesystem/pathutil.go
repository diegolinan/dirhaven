package filesystem

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// openPath returns an os.Root handle and a name relative to it. os.Root ensures
// that symlink/junction traversal cannot leave the chosen allowed filesystem
// root, including when concurrent directory changes occur after validation.
func (s *Service) openPath(input string) (*os.Root, string, error) {
	if strings.TrimSpace(input) == "" {
		return nil, "", errors.New("path required")
	}
	abs, err := filepath.Abs(input)
	if err != nil {
		return nil, "", err
	}
	abs = filepath.Clean(abs)
	roots, err := s.availableRoots()
	if err != nil {
		return nil, "", err
	}
	// Preserve the literal path for dynamically discovered Windows drive
	// roots (e.g. X:\\). Canonicalizing those paths first would erase the
	// drive-letter identity required by a live mount.
	selected, relative := matchingRoot(roots, abs)
	if selected == "" {
		// Users may address a validated root via an OS-equivalent spelling:
		// macOS /var -> /private/var or Windows short 8.3 path segments.
		// Resolve the deepest existing ancestor, then append missing names.
		// The final operation still runs through os.Root for confinement.
		canonical, err := canonicalInputPath(abs)
		if err != nil {
			return nil, "", fmt.Errorf("access denied: cannot resolve path alias")
		}
		selected, relative = matchingRoot(roots, canonical)
	}
	if selected == "" {
		return nil, "", fmt.Errorf("access denied: %q is outside allowed roots", input)
	}
	r, err := os.OpenRoot(selected)
	if err != nil {
		return nil, "", err
	}
	return r, relative, nil
}

func matchingRoot(roots []string, candidate string) (selected, relative string) {
	for _, root := range roots {
		rel, err := filepath.Rel(root, candidate)
		if err != nil || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			continue
		}
		if len(root) > len(selected) {
			selected, relative = root, rel
		}
	}
	return selected, relative
}

// canonicalInputPath resolves existing path aliases, but does not require the
// final file/directory to exist. It never grants access on its own: openPath
// must still match an allowed root and os.Root performs the actual operation.
func canonicalInputPath(absolute string) (string, error) {
	current := filepath.Clean(absolute)
	var missing []string
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			return filepath.Clean(resolved), nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", err
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}

func (s *Service) openParent(input string) (*os.Root, string, error) {
	r, name, err := s.openPath(input)
	if err != nil {
		return nil, "", err
	}
	if name == "." {
		r.Close()
		return nil, "", errors.New("cannot modify an allowed root")
	}
	return r, name, nil
}
