package filesystem

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/pmezard/go-difflib/difflib"
)

type Entry struct {
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	Children *[]Entry `json:"children,omitempty"`
}

func (s *Service) readDir(input string) ([]os.DirEntry, error) {
	root, rel, err := s.openPath(input)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	dir, err := root.Open(rel)
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	return dir.ReadDir(-1)
}

func (s *Service) ReadMultipleFiles(paths []string) (string, error) {
	if len(paths) == 0 {
		return "", errors.New("at least one path required")
	}
	var results []string
	for _, p := range paths {
		content, err := s.ReadTextFile(p, 0, 0)
		if err != nil {
			detail := err.Error()
			if errors.Is(err, fs.ErrNotExist) {
				detail = fmt.Sprintf("ENOENT: no such file or directory, open '%s'", p)
			}
			results = append(results, fmt.Sprintf("%s: Error - %s", p, detail))
			continue
		}
		results = append(results, fmt.Sprintf("%s:\n%s\n", p, content))
	}
	return strings.Join(results, "\n---\n"), nil
}

func (s *Service) CreateDirectory(input string) error {
	r, rel, err := s.openParent(input)
	if err != nil {
		return err
	}
	defer r.Close()
	return r.MkdirAll(rel, 0755)
}

func (s *Service) WriteFile(input, content string) error {
	r, rel, err := s.openParent(input)
	if err != nil {
		return err
	}
	defer r.Close()
	// Exclusive creation prevents a pre-existing symlink from being followed.
	f, err := r.OpenFile(rel, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err == nil {
		if _, err = f.Write([]byte(content)); err != nil {
			f.Close()
			r.Remove(rel)
			return err
		}
		return f.Close()
	}
	if !errors.Is(err, fs.ErrExist) {
		return err
	}
	fi, err := r.Lstat(rel)
	if err != nil {
		return err
	}
	if !fi.Mode().IsRegular() {
		return fmt.Errorf("refusing to overwrite non-regular file: %s", input)
	}
	tmp, err := randomTempName(filepath.Dir(rel), filepath.Base(rel))
	if err != nil {
		return err
	}
	f, err = r.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, fi.Mode().Perm())
	if err != nil {
		return err
	}
	defer r.Remove(tmp)
	if _, err = f.Write([]byte(content)); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	// The temp and destination are both constrained by the same os.Root.
	if err = r.Rename(tmp, rel); err != nil {
		return err
	}
	return nil
}

func randomTempName(dir, base string) (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return filepath.Join(dir, "."+base+"."+hex.EncodeToString(b[:])+".tmp"), nil
}

func (s *Service) MoveFile(src, dst string) error {
	if src == dst {
		return fmt.Errorf("destination already exists: %s", dst)
	}
	srcRoot, srcRel, err := s.openParent(src)
	if err != nil {
		return err
	}
	defer srcRoot.Close()
	dstRoot, dstRel, err := s.openParent(dst)
	if err != nil {
		return err
	}
	defer dstRoot.Close()
	if _, err := srcRoot.Lstat(srcRel); err != nil {
		return err
	}
	if _, err := dstRoot.Lstat(dstRel); err == nil {
		return fmt.Errorf("destination already exists: %s", dst)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if srcRoot.Name() != dstRoot.Name() {
		return errors.New("cross-root move not yet implemented; refusing non-atomic move")
	}
	return srcRoot.Rename(srcRel, dstRel)
}

func (s *Service) ListDirectoryWithSizes(input, sortBy string) (string, error) {
	entries, err := s.readDir(input)
	if err != nil {
		return "", err
	}
	type item struct {
		Name  string
		Size  int64
		IsDir bool
	}
	items := make([]item, 0, len(entries))
	var total int64
	var files, dirs int
	for _, e := range entries {
		size := int64(0)
		if fi, err := e.Info(); err == nil {
			size = fi.Size()
		}
		isDir := e.IsDir()
		items = append(items, item{Name: e.Name(), Size: size, IsDir: isDir})
		if isDir {
			dirs++
		} else {
			files++
			total += size
		}
	}
	if sortBy == "size" {
		sort.SliceStable(items, func(i, j int) bool { return items[i].Size > items[j].Size })
	} else {
		sort.Slice(items, func(i, j int) bool { return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name) })
	}
	var out []string
	for _, item := range items {
		kind := "[FILE]"
		size := fmt.Sprintf("%10s", formatSize(item.Size))
		if item.IsDir {
			kind = "[DIR]"
			size = ""
		}
		out = append(out, fmt.Sprintf("%s %-30s %s", kind, item.Name, size))
	}
	out = append(out, "", fmt.Sprintf("Total: %d files, %d directories", files, dirs), "Combined size: "+formatSize(total))
	return strings.Join(out, "\n"), nil
}

func formatSize(size int64) string {
	if size <= 0 {
		return "0 B"
	}
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	}
	units := []string{"B", "KB", "MB", "GB", "TB"}
	i := int(math.Floor(math.Log(float64(size)) / math.Log(1024)))
	if i >= len(units) {
		i = len(units) - 1
	}
	return fmt.Sprintf("%.2f %s", float64(size)/math.Pow(1024, float64(i)), units[i])
}

func matchesPattern(pattern, relative string) bool {
	relative = filepath.ToSlash(relative)
	pattern = filepath.ToSlash(pattern)
	ok, err := doublestar.Match(pattern, relative)
	if err != nil {
		return false
	}
	return ok
}
func excluded(relative string, patterns []string) bool {
	relative = filepath.ToSlash(relative)
	base := path.Base(relative)
	for _, p := range patterns {
		if matchesPattern(p, relative) {
			return true
		}
		if !strings.ContainsAny(p, "/*?[]") && (p == base || strings.Contains("/"+relative+"/", "/"+p+"/")) {
			return true
		}
	}
	return false
}

func (s *Service) Tree(input string, excludes []string) (string, error) {
	r, rel, err := s.openPath(input)
	if err != nil {
		return "", err
	}
	defer r.Close()
	var walk func(string, string) ([]Entry, error)
	walk = func(current, relative string) ([]Entry, error) {
		d, err := r.Open(current)
		if err != nil {
			return nil, err
		}
		entries, err := d.ReadDir(-1)
		d.Close()
		if err != nil {
			return nil, err
		}
		out := make([]Entry, 0, len(entries))
		for _, e := range entries {
			entryRel := e.Name()
			if relative != "" {
				entryRel = filepath.Join(relative, e.Name())
			}
			if excluded(entryRel, excludes) {
				continue
			}
			typ := "file"
			entry := Entry{Name: e.Name(), Type: typ}
			if e.IsDir() {
				entry.Type = "directory"
				children, err := walk(filepath.Join(current, e.Name()), entryRel)
				if err != nil {
					return nil, err
				}
				if children == nil {
					children = []Entry{}
				}
				entry.Children = &children
			}
			out = append(out, entry)
		}
		return out, nil
	}
	entries, err := walk(rel, "")
	if err != nil {
		return "", err
	}
	if entries == nil {
		entries = []Entry{}
	}
	b, err := json.MarshalIndent(entries, "", "  ")
	return string(b), err
}

func (s *Service) SearchFiles(input, pattern string, excludes []string) (string, error) {
	if pattern == "" {
		return "", errors.New("search pattern required")
	}
	if !doublestar.ValidatePattern(filepath.ToSlash(pattern)) {
		return "", fmt.Errorf("invalid glob pattern: %s", pattern)
	}
	r, rel, err := s.openPath(input)
	if err != nil {
		return "", err
	}
	defer r.Close()
	var results []string
	var walk func(string, string) error
	walk = func(current, relative string) error {
		d, err := r.Open(current)
		if err != nil {
			return err
		}
		entries, err := d.ReadDir(-1)
		d.Close()
		if err != nil {
			return err
		}
		for _, e := range entries {
			entryRel := e.Name()
			if relative != "" {
				entryRel = filepath.Join(relative, e.Name())
			}
			if excluded(entryRel, excludes) {
				continue
			}
			if matchesPattern(pattern, entryRel) {
				results = append(results, filepath.Join(input, entryRel))
			}
			if e.IsDir() {
				if err := walk(filepath.Join(current, e.Name()), entryRel); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := walk(rel, ""); err != nil {
		return "", err
	}
	if len(results) == 0 {
		return "No matches found", nil
	}
	return strings.Join(results, "\n"), nil
}

func (s *Service) GetFileInfo(input string) (string, error) {
	r, rel, err := s.openPath(input)
	if err != nil {
		return "", err
	}
	defer r.Close()
	fi, err := r.Stat(rel)
	if err != nil {
		return "", err
	}
	type item struct{ k, v string }
	created, accessed := fileAccessTimes(fi)
	items := []item{
		{"size", fmt.Sprint(fi.Size())},
		{"created", knownTimestamp(created)},
		{"modified", knownTimestamp(fi.ModTime())},
		{"accessed", knownTimestamp(accessed)},
		{"isDirectory", fmt.Sprint(fi.IsDir())},
		{"isFile", fmt.Sprint(fi.Mode().IsRegular())},
		{"permissions", fmt.Sprintf("%03o", fi.Mode().Perm())},
	}
	lines := make([]string, 0, len(items))
	for _, it := range items {
		lines = append(lines, it.k+": "+it.v)
	}
	return strings.Join(lines, "\n"), nil
}

type Edit struct {
	OldText string `json:"oldText"`
	NewText string `json:"newText"`
}

func (s *Service) EditFile(input string, edits []Edit, dryRun bool) (string, error) {
	original, err := s.ReadTextFile(input, 0, 0)
	if err != nil {
		return "", err
	}
	old := strings.ReplaceAll(original, "\r\n", "\n")
	modified := old
	for _, e := range edits {
		search := strings.ReplaceAll(e.OldText, "\r\n", "\n")
		replacement := strings.ReplaceAll(e.NewText, "\r\n", "\n")
		if search == "" {
			return "", errors.New("empty oldText not supported")
		}
		if strings.Contains(modified, search) {
			modified = strings.Replace(modified, search, replacement, 1)
			continue
		}
		// Match the Node server's whitespace-insensitive fallback for
		// line-oriented edits while preserving original indentation.
		oldLines := strings.Split(search, "\n")
		lines := strings.Split(modified, "\n")
		found := false
		for start := 0; start+len(oldLines) <= len(lines); start++ {
			matching := true
			for j, oldLine := range oldLines {
				if strings.TrimSpace(lines[start+j]) != strings.TrimSpace(oldLine) {
					matching = false
					break
				}
			}
			if !matching {
				continue
			}
			indent := leadingWhitespace(lines[start])
			newLines := strings.Split(replacement, "\n")
			for j, line := range newLines {
				if j == 0 {
					newLines[j] = indent + strings.TrimLeft(line, " \t")
					continue
				}
				if j < len(oldLines) && leadingWhitespace(oldLines[j]) != "" && leadingWhitespace(line) != "" {
					offset := len(leadingWhitespace(line)) - len(leadingWhitespace(oldLines[j]))
					if offset < 0 {
						offset = 0
					}
					newLines[j] = indent + strings.Repeat(" ", offset) + strings.TrimLeft(line, " \t")
				}
			}
			combined := make([]string, 0, len(lines)-len(oldLines)+len(newLines))
			combined = append(combined, lines[:start]...)
			combined = append(combined, newLines...)
			combined = append(combined, lines[start+len(oldLines):]...)
			modified = strings.Join(combined, "\n")
			found = true
			break
		}
		if !found {
			return "", fmt.Errorf("could not find exact match for edit:\n%s", e.OldText)
		}
	}
	ud := difflib.UnifiedDiff{
		A: splitUnifiedLines(old), B: splitUnifiedLines(modified),
		FromFile: input, ToFile: input, FromDate: "original", ToDate: "modified", Context: 3,
	}
	unified, err := difflib.GetUnifiedDiffString(ud)
	if err != nil {
		return "", err
	}
	if !dryRun && modified != old {
		if err := s.WriteFile(input, modified); err != nil {
			return "", err
		}
	}
	diff := "Index: " + input + "\n" + "===================================================================\n" + unified
	ticks := 3
	for strings.Contains(diff, strings.Repeat("`", ticks)) {
		ticks++
	}
	fence := strings.Repeat("`", ticks)
	return fence + "diff\n" + diff + fence + "\n\n", nil
}

func (s *Service) ReadBinary(input string) ([]byte, error) {
	r, rel, err := s.openPath(input)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	f, err := r.Open(rel)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("not a regular file: %s", input)
	}
	// Tool responses should not accidentally exhaust the MCP bridge memory.
	if fi.Size() > 32<<20 {
		return nil, errors.New("file exceeds 32 MiB MCP media response limit")
	}
	return io.ReadAll(io.LimitReader(f, (32<<20)+1))
}

func leadingWhitespace(s string) string {
	return s[:len(s)-len(strings.TrimLeft(s, " \t"))]
}

func knownTimestamp(t time.Time) string {
	if t.IsZero() {
		return "unknown"
	}
	local := t.In(time.Local)
	return local.Format("Mon Jan 02 2006 15:04:05 GMT-0700") + " (" + localZoneLabel(local) + ")"
}

func splitUnifiedLines(text string) []string {
	if text == "" {
		return []string{}
	}
	lines := strings.SplitAfter(text, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) > 0 && !strings.HasSuffix(lines[len(lines)-1], "\n") {
		lines[len(lines)-1] += "\n"
	}
	return lines
}
