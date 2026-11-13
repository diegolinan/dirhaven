package filesystem

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const (
	maxBatchFiles        = 8
	maxBatchQueries      = 6
	maxBatchMatches      = 40
	defaultBatchMatches  = 12
	maxBatchContextLines = 3
	maxBatchSourceBytes  = 64 << 20
)

// batchCursor points to the FIRST unreturned match, inclusive. It is bound
// to the snapshot_sha256 of both the inputs and the actual source bytes.
type batchCursor struct {
	FileIndex int `json:"file_index"`
	Line      int `json:"line"`
}

type batchSource struct {
	Path string `json:"path"`
	sourceInfo
	TotalMatches   int `json:"total_matches"`
	FirstMatchLine int `json:"first_match_line,omitempty"`
	LastMatchLine  int `json:"last_match_line,omitempty"`
}

type batchMatch struct {
	FileIndex      int          `json:"file_index"`
	MatchLine      int          `json:"match_line"`
	MatchedQueries []string     `json:"matched_queries"`
	Lines          []sourceLine `json:"lines"`
}

type batchSearchPage struct {
	SnapshotSHA256 string        `json:"snapshot_sha256"`
	Queries        []string      `json:"queries"`
	CaseSensitive  bool          `json:"case_sensitive"`
	Direction      string        `json:"direction"`
	ContextLines   int           `json:"context_lines"`
	Sources        []batchSource `json:"sources"`
	TotalMatches   int           `json:"total_matches"`
	Matches        []batchMatch  `json:"matches"`
	NextCursor     *batchCursor  `json:"next_cursor,omitempty"`
}

func validateBatchParams(paths, queries []string, contextLines, maxMatches int, direction string, cursor *batchCursor, expected string) ([]string, string, int, error) {
	if len(paths) == 0 || len(paths) > maxBatchFiles {
		return nil, "", 0, fmt.Errorf("paths must contain 1-%d explicit files", maxBatchFiles)
	}
	seenPaths := make(map[string]bool, len(paths))
	for _, p := range paths {
		if strings.TrimSpace(p) == "" {
			return nil, "", 0, errors.New("paths cannot contain empty names")
		}
		abs, err := filepath.Abs(p)
		if err != nil {
			return nil, "", 0, err
		}
		key := strings.ToLower(filepath.Clean(abs))
		if seenPaths[key] {
			return nil, "", 0, errors.New("duplicate source paths are not allowed")
		}
		seenPaths[key] = true
	}
	if len(queries) == 0 || len(queries) > maxBatchQueries {
		return nil, "", 0, fmt.Errorf("queries must contain 1-%d literal strings", maxBatchQueries)
	}
	normalized := make([]string, 0, len(queries))
	seen := make(map[string]bool, len(queries))
	for _, query := range queries {
		if strings.TrimSpace(query) == "" || len(query) > 256 || !utf8.ValidString(query) || strings.IndexByte(query, 0) >= 0 {
			return nil, "", 0, errors.New("each literal query must be nonempty valid UTF-8, <= 256 bytes, without NUL")
		}
		key := strings.ToLower(query)
		if seen[key] {
			return nil, "", 0, errors.New("duplicate queries are not allowed")
		}
		seen[key] = true
		normalized = append(normalized, key)
	}
	if contextLines < 0 || contextLines > maxBatchContextLines {
		return nil, "", 0, fmt.Errorf("context_lines must be 0-%d", maxBatchContextLines)
	}
	if maxMatches == 0 {
		maxMatches = defaultBatchMatches
	}
	if maxMatches < 1 || maxMatches > maxBatchMatches {
		return nil, "", 0, fmt.Errorf("max_matches must be 1-%d", maxBatchMatches)
	}
	if direction == "" {
		direction = "forward"
	}
	if direction != "forward" && direction != "reverse" {
		return nil, "", 0, errors.New("direction must be forward or reverse")
	}
	if cursor != nil {
		if cursor.FileIndex < 0 || cursor.FileIndex >= len(paths) || cursor.Line < 1 {
			return nil, "", 0, errors.New("cursor outside requested source files")
		}
		if expected == "" {
			return nil, "", 0, errors.New("expected_snapshot_sha256 is required when resuming a cursor")
		}
	}
	return normalized, direction, maxMatches, nil
}

// SearchTextFiles reads each authorized file under os.Root. The first pass
// counts ALL matching lines and fixes content SHA-256 versions; the second
// expands only the at-most-41 candidate matches into exact context windows.
// No content is returned if any file changes between the two passes.
func (s *Service) SearchTextFilesContext(ctx context.Context, paths, queries []string, contextLines, maxMatches int, caseSensitive bool, direction string, cursor *batchCursor, expectedSnapshot string) (string, error) {
	normalized, direction, maxMatches, err := validateBatchParams(paths, queries, contextLines, maxMatches, direction, cursor, expectedSnapshot)
	if err != nil {
		return "", err
	}
	needles := append([]string(nil), queries...)
	if !caseSensitive {
		needles = normalized
	}
	sources := make([]batchSource, len(paths))
	candidates := make([]batchMatch, 0, maxMatches+1)
	totalMatches, eligible := 0, 0
	var totalBytes int64

	for index, filePath := range paths {
		src := batchSource{Path: filePath}
		info, err := s.scanSource(ctx, filePath, func(line sourceLine) error {
			candidateText := line.Text
			if !caseSensitive {
				candidateText = strings.ToLower(candidateText)
			}
			var matched []string
			for i, needle := range needles {
				if strings.Contains(candidateText, needle) {
					matched = append(matched, queries[i])
				}
			}
			if len(matched) == 0 {
				return nil
			}
			src.TotalMatches++
			if src.FirstMatchLine == 0 {
				src.FirstMatchLine = line.Number
			}
			src.LastMatchLine = line.Number
			totalMatches++

			if cursor != nil {
				if direction == "forward" && (index < cursor.FileIndex || (index == cursor.FileIndex && line.Number < cursor.Line)) {
					return nil
				}
				if direction == "reverse" && (index > cursor.FileIndex || (index == cursor.FileIndex && line.Number > cursor.Line)) {
					return nil
				}
			}
			eligible++
			match := batchMatch{FileIndex: index, MatchLine: line.Number, MatchedQueries: matched}
			if direction == "forward" {
				if len(candidates) < maxMatches+1 {
					candidates = append(candidates, match)
				}
			} else {
				candidates = append(candidates, match)
				if len(candidates) > maxMatches+1 {
					copy(candidates, candidates[1:])
					candidates = candidates[:maxMatches+1]
				}
			}
			return nil
		})
		if err != nil {
			return "", err
		}
		totalBytes += info.FileBytes
		if totalBytes > maxBatchSourceBytes {
			return "", fmt.Errorf("combined source files exceed %d bytes; narrow paths", maxBatchSourceBytes)
		}
		src.sourceInfo = info
		sources[index] = src
	}

	// A cursor is coupled to the source revisions, query, path order,
	// direction and requested context. Reusing it for a different search fails.
	snapshotInput, err := json.Marshal(struct {
		Paths         []string `json:"paths"`
		Queries       []string `json:"queries"`
		CaseSensitive bool     `json:"case_sensitive"`
		Direction     string   `json:"direction"`
		ContextLines  int      `json:"context_lines"`
		Hashes        []string `json:"hashes"`
	}{
		Paths: paths, Queries: queries, CaseSensitive: caseSensitive,
		Direction: direction, ContextLines: contextLines,
		Hashes: func() []string {
			a := make([]string, len(sources))
			for i := range sources {
				a[i] = sources[i].FileSHA256
			}
			return a
		}(),
	})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(snapshotInput)
	snapshot := hex.EncodeToString(digest[:])
	if err := validateExpectedVersion(expectedSnapshot, snapshot); err != nil {
		return "", fmt.Errorf("batch search snapshot: %w", err)
	}

	if direction == "reverse" {
		for left, right := 0, len(candidates)-1; left < right; left, right = left+1, right-1 {
			candidates[left], candidates[right] = candidates[right], candidates[left]
		}
	}

	// Repeat the confined, bounded scan for only files with candidate windows,
	// verifying the second digest to prevent torn multi-file evidence.
	for index, filePath := range paths {
		var positions []int
		for j := range candidates {
			if candidates[j].FileIndex == index {
				positions = append(positions, j)
			}
		}
		// Revalidate even files without returned matches. A formerly empty
		// file might have changed and gained a match between passes.
		info, err := s.scanSource(ctx, filePath, func(line sourceLine) error {
			for _, j := range positions {
				hit := &candidates[j]
				start, end := hit.MatchLine-contextLines, hit.MatchLine+contextLines
				if line.Number >= start && line.Number <= end {
					hit.Lines = append(hit.Lines, line)
				}
			}
			return nil
		})
		if err != nil {
			return "", err
		}
		if info.FileSHA256 != sources[index].FileSHA256 || info.FileBytes != sources[index].FileBytes || info.TotalLines != sources[index].TotalLines {
			return "", errors.New("source changed during batch context expansion; retry from start")
		}
	}

	page := batchSearchPage{
		SnapshotSHA256: snapshot, Queries: queries, CaseSensitive: caseSensitive,
		Direction: direction, ContextLines: contextLines, Sources: sources, TotalMatches: totalMatches,
	}
	count := len(candidates)
	if count > maxMatches {
		count = maxMatches
	}
	// The response cap may force smaller pages than max_matches. Never truncate
	// any source line or skip any match. A cursor points to the first omitted hit.
	for {
		page.Matches = candidates[:count]
		page.NextCursor = nil
		if eligible > count {
			if count >= len(candidates) {
				return "", errors.New("internal pagination inconsistency")
			}
			omitted := candidates[count]
			page.NextCursor = &batchCursor{FileIndex: omitted.FileIndex, Line: omitted.MatchLine}
		}
		out, err := json.Marshal(page)
		if err != nil {
			return "", err
		}
		if len(out) <= maxSelectionOutputBytes {
			return string(out), nil
		}
		if count == 0 {
			return "", errors.New("batch metadata exceeds response budget; reduce paths or query size")
		}
		if count == 1 {
			return "", errors.New("one match and its context exceed response budget; reduce context_lines or use a byte-range reader")
		}
		count--
	}
}
