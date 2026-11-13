package filesystem

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

const (
	// v1 deliberately scopes these tools to individual text files. Existing
	// search_files can narrow the candidate filenames first.
	maxSelectedFileBytes    int64 = 32 << 20
	maxPhysicalLineBytes          = 1 << 20
	maxSelectionOutputBytes       = 16 << 10
	defaultLinesPerPage           = 60
	maxLinesPerPage               = 300
	defaultSearchMatches          = 20
	maxSearchMatches              = 100
)

type sourceLine struct {
	Number int    `json:"number"`
	Text   string `json:"text"`
	EOL    string `json:"eol"` // lf, crlf, none: reconstruction is exact
}

type sourceInfo struct {
	FileSHA256 string `json:"file_sha256"`
	FileBytes  int64  `json:"file_bytes"`
	TotalLines int    `json:"total_lines"`
}

type lineSelection struct {
	sourceInfo
	StartLine int          `json:"start_line"`
	Lines     []sourceLine `json:"lines"`
	NextLine  int          `json:"next_line,omitempty"`
}

type searchSelection struct {
	sourceInfo
	Query         string       `json:"query"`
	CaseSensitive bool         `json:"case_sensitive"`
	StartLine     int          `json:"start_line"`
	TotalMatches  int          `json:"total_matches"`
	Matches       []sourceLine `json:"matches"`
	NextLine      int          `json:"next_line,omitempty"`
}

// readPhysicalLine preserves the original newline type but rejects pathological
// lines instead of allocating unlimited memory or truncating evidence silently.
func readPhysicalLine(reader *bufio.Reader) ([]byte, bool, error) {
	var result []byte
	for {
		part, err := reader.ReadSlice('\n')
		if len(result)+len(part) > maxPhysicalLineBytes {
			return nil, false, fmt.Errorf("source contains a line larger than %d bytes; use a byte-oriented tool", maxPhysicalLineBytes)
		}
		result = append(result, part...)
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if err == io.EOF && len(result) == 0 {
			return nil, true, nil
		}
		if err != nil && err != io.EOF {
			return nil, false, err
		}
		return result, false, nil
	}
}

func decodeSourceLine(raw []byte, number int) (sourceLine, error) {
	line := sourceLine{Number: number, EOL: "none"}
	if len(raw) > 0 && raw[len(raw)-1] == '\n' {
		line.EOL = "lf"
		raw = raw[:len(raw)-1]
		if len(raw) > 0 && raw[len(raw)-1] == '\r' {
			line.EOL = "crlf"
			raw = raw[:len(raw)-1]
		}
	}
	if !utf8.Valid(raw) || bytes.IndexByte(raw, 0) >= 0 {
		return sourceLine{}, fmt.Errorf("line %d is not valid UTF-8 text", number)
	}
	line.Text = string(raw)
	return line, nil
}

// scanSource reads through os.Root to preserve root confinement, hashes the
// exact file bytes and checks for concurrent metadata changes. It only holds
// one bounded physical line in memory at a time.
func (s *Service) scanSource(ctx context.Context, input string, visit func(sourceLine) error) (sourceInfo, error) {
	root, rel, err := s.openPath(input)
	if err != nil {
		return sourceInfo{}, err
	}
	defer root.Close()
	file, err := root.Open(rel)
	if err != nil {
		return sourceInfo{}, err
	}
	defer file.Close()
	before, err := file.Stat()
	if err != nil {
		return sourceInfo{}, err
	}
	if !before.Mode().IsRegular() {
		return sourceInfo{}, fmt.Errorf("not a regular text file: %s", input)
	}
	if before.Size() > maxSelectedFileBytes {
		return sourceInfo{}, fmt.Errorf("file exceeds the %d byte selection limit; narrow the source", maxSelectedFileBytes)
	}
	h := sha256.New()
	src := &countedReader{r: io.TeeReader(io.LimitReader(file, maxSelectedFileBytes+1), h)}
	reader := bufio.NewReaderSize(src, 64<<10)
	metadata := sourceInfo{FileBytes: before.Size()}
	for {
		if err := ctx.Err(); err != nil {
			return sourceInfo{}, err
		}
		raw, done, err := readPhysicalLine(reader)
		if err != nil {
			return sourceInfo{}, err
		}
		if done {
			break
		}
		metadata.TotalLines++
		line, err := decodeSourceLine(raw, metadata.TotalLines)
		if err != nil {
			return sourceInfo{}, err
		}
		if err := visit(line); err != nil {
			return sourceInfo{}, err
		}
	}
	after, err := file.Stat()
	if err != nil {
		return sourceInfo{}, err
	}
	if src.n > maxSelectedFileBytes || src.n != before.Size() ||
		before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return sourceInfo{}, errors.New("source changed during reading or exceeded the size limit; retry")
	}
	metadata.FileSHA256 = hex.EncodeToString(h.Sum(nil))
	return metadata, nil
}

type countedReader struct {
	r io.Reader
	n int64
}

func (r *countedReader) Read(buf []byte) (int, error) {
	n, err := r.r.Read(buf)
	r.n += int64(n)
	return n, err
}

func validateExpectedVersion(expected, actual string) error {
	if expected == "" {
		return nil
	}
	if len(expected) != 64 || expected != strings.ToLower(expected) {
		return errors.New("expected_sha256 must be a lowercase 64-character SHA-256 hex string")
	}
	if _, err := hex.DecodeString(expected); err != nil {
		return errors.New("expected_sha256 is not valid hex")
	}
	if expected != actual {
		return errors.New("source revision mismatch; restart pagination from line 1")
	}
	return nil
}

// ReadFileLines returns only complete, numbered physical lines plus EOL metadata.
// It never silently drops content; NextLine identifies the first unreturned line.
func (s *Service) ReadFileLines(path string, startLine, limit int, expectedSHA256 string) (string, error) {
	return s.ReadFileLinesContext(context.Background(), path, startLine, limit, expectedSHA256)
}

func (s *Service) ReadFileLinesContext(ctx context.Context, path string, startLine, limit int, expectedSHA256 string) (string, error) {
	if startLine < 1 {
		return "", errors.New("start_line must be >= 1")
	}
	if limit == 0 {
		limit = defaultLinesPerPage
	}
	if limit < 1 || limit > maxLinesPerPage {
		return "", fmt.Errorf("limit must be between 1 and %d", maxLinesPerPage)
	}
	lines := make([]sourceLine, 0, limit)
	meta, err := s.scanSource(ctx, path, func(line sourceLine) error {
		if line.Number >= startLine && len(lines) < limit {
			lines = append(lines, line)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if err := validateExpectedVersion(expectedSHA256, meta.FileSHA256); err != nil {
		return "", err
	}
	page := lineSelection{sourceInfo: meta, StartLine: startLine, Lines: lines}
	for {
		if len(page.Lines) > 0 && page.Lines[len(page.Lines)-1].Number < meta.TotalLines {
			page.NextLine = page.Lines[len(page.Lines)-1].Number + 1
		} else if len(page.Lines) == 0 && startLine <= meta.TotalLines {
			page.NextLine = startLine
		} else {
			page.NextLine = 0
		}
		out, err := json.Marshal(page)
		if err != nil {
			return "", err
		}
		if len(out) <= maxSelectionOutputBytes {
			return string(out), nil
		}
		if len(page.Lines) <= 1 {
			return "", errors.New("one source line exceeds the safe response cap; use a byte-range reader")
		}
		page.Lines = page.Lines[:len(page.Lines)-1]
	}
}

// SearchTextFile performs literal, case-optional matching, not regex or AI
// summarization. It counts every matching line even if only a page is returned.
func (s *Service) SearchTextFile(path, query string, startLine, maxMatches int, caseSensitive bool, expectedSHA256 string) (string, error) {
	return s.SearchTextFileContext(context.Background(), path, query, startLine, maxMatches, caseSensitive, expectedSHA256)
}

func (s *Service) SearchTextFileContext(ctx context.Context, path, query string, startLine, maxMatches int, caseSensitive bool, expectedSHA256 string) (string, error) {
	if strings.TrimSpace(query) == "" || len(query) > 256 {
		return "", errors.New("query must be a nonempty literal of at most 256 bytes")
	}
	if !utf8.ValidString(query) || strings.IndexByte(query, 0) >= 0 {
		return "", errors.New("query must be valid UTF-8 text")
	}
	if startLine < 1 {
		return "", errors.New("start_line must be >= 1")
	}
	if maxMatches == 0 {
		maxMatches = defaultSearchMatches
	}
	if maxMatches < 1 || maxMatches > maxSearchMatches {
		return "", fmt.Errorf("max_matches must be between 1 and %d", maxSearchMatches)
	}
	matches := make([]sourceLine, 0, maxMatches)
	totalMatches := 0
	hasLater := false
	needle := query
	if !caseSensitive {
		needle = strings.ToLower(query)
	}
	meta, err := s.scanSource(ctx, path, func(line sourceLine) error {
		candidate := line.Text
		if !caseSensitive {
			candidate = strings.ToLower(candidate)
		}
		if strings.Contains(candidate, needle) {
			totalMatches++
			if line.Number >= startLine {
				if len(matches) < maxMatches {
					matches = append(matches, line)
				} else {
					hasLater = true
				}
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if err := validateExpectedVersion(expectedSHA256, meta.FileSHA256); err != nil {
		return "", err
	}
	page := searchSelection{
		sourceInfo: meta, Query: query, CaseSensitive: caseSensitive,
		StartLine: startLine, Matches: matches, TotalMatches: totalMatches,
	}
	for {
		page.NextLine = 0
		if len(page.Matches) > 0 {
			last := page.Matches[len(page.Matches)-1].Number
			if hasLater || len(page.Matches) < len(matches) {
				page.NextLine = last + 1
			}
		}
		out, err := json.Marshal(page)
		if err != nil {
			return "", err
		}
		if len(out) <= maxSelectionOutputBytes {
			return string(out), nil
		}
		if len(page.Matches) <= 1 {
			return "", errors.New("one matching source line exceeds the safe response cap; use read_file_lines for a narrower range")
		}
		page.Matches = page.Matches[:len(page.Matches)-1]
	}
}
