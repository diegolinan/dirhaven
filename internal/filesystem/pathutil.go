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
	selected, relative := "", ""
	for _, root := range roots {
		rel, err := filepath.Rel(root, abs)
		if err != nil || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			continue
		}
		if len(root) > len(selected) {
			selected, relative = root, rel
		}
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
