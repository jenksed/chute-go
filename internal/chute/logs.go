package chute

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

const (
	DefaultLogContext = 5
	maxLogLines       = 20_000
)

type LogLine struct {
	Line    int                  `json:"line"`
	Text    string               `json:"text"`
	Matches []EvidenceIdentifier `json:"matches,omitempty"`
}

type LogWindow struct {
	Source             string               `json:"source"`
	StartLine          int                  `json:"start_line"`
	EndLine            int                  `json:"end_line"`
	Tier               EvidenceTier         `json:"tier"`
	MatchedIdentifiers []EvidenceIdentifier `json:"matched_identifiers"`
	Lines              []LogLine            `json:"lines"`
}

type LogReadError struct {
	Source string `json:"source"`
	Error  string `json:"error"`
}

type LogEvidence struct {
	Windows       []LogWindow             `json:"windows,omitempty"`
	MatchedLines  int                     `json:"matched_lines"`
	MatchedByTier map[EvidenceTier]int    `json:"matched_by_tier"`
	IncludedLines int                     `json:"included_lines"`
	ContextLines  int                     `json:"context_lines"`
	Truncated     bool                    `json:"truncated"`
	Errors        []LogReadError          `json:"read_errors,omitempty"`
}

type logHit struct {
	line        int
	identifiers []EvidenceIdentifier
}

type logInterval struct {
	start       int
	end         int
	tier        EvidenceTier
	identifiers []EvidenceIdentifier
}

func ExtractLogs(root string, inventory []InventoryEntry, identifiers []EvidenceIdentifier, contextLines int) LogEvidence {
	if contextLines < 0 {
		contextLines = 0
	}

	evidence := LogEvidence{
		ContextLines:  contextLines,
		MatchedByTier: map[EvidenceTier]int{TierPrimary: 0, TierSecondary: 0, TierContextual: 0},
	}
	for _, entry := range inventory {
		if entry.Category != "log" {
			continue
		}
		scanLogFile(root, entry, identifiers, contextLines, &evidence)
	}

	return evidence
}

func scanLogFile(root string, entry InventoryEntry, identifiers []EvidenceIdentifier, contextLines int, evidence *LogEvidence) {
	path := physicalPath(root, entry)
	hits, err := findLogHits(path, identifiers)
	if err != nil {
		evidence.Errors = append(evidence.Errors, LogReadError{Source: entry.Path, Error: err.Error()})
		return
	}
	if len(hits) == 0 {
		return
	}

	evidence.MatchedLines += len(hits)
	for _, hit := range hits {
		tier := strongestIdentifierTier(hit.identifiers)
		evidence.MatchedByTier[tier]++
	}

	intervals := mergeLogIntervals(hits, contextLines)
	windows, err := readLogWindows(path, entry.Path, intervals, hits, maxLogLines-evidence.IncludedLines)
	if err != nil {
		evidence.Errors = append(evidence.Errors, LogReadError{Source: entry.Path, Error: err.Error()})
		return
	}

	for _, window := range windows {
		evidence.IncludedLines += len(window.Lines)
		evidence.Windows = append(evidence.Windows, window)
		if evidence.IncludedLines >= maxLogLines {
			evidence.Truncated = true
			return
		}
	}
}

func findLogHits(path string, identifiers []EvidenceIdentifier) ([]logHit, error) {
	reader, err := openLogReader(path)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	scanner := newLogScanner(reader)
	var hits []logHit
	lineNumber := 0

	for scanner.Scan() {
		lineNumber++
		matched := matchedIdentifiers(scanner.Text(), identifiers)
		if len(matched) > 0 {
			hits = append(hits, logHit{line: lineNumber, identifiers: matched})
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan failed: %w", err)
	}
	return hits, nil
}

func mergeLogIntervals(hits []logHit, contextLines int) []logInterval {
	if len(hits) == 0 {
		return nil
	}

	var intervals []logInterval
	for _, hit := range hits {
		start := hit.line - contextLines
		if start < 1 {
			start = 1
		}
		end := hit.line + contextLines
		tier := strongestIdentifierTier(hit.identifiers)

		if len(intervals) == 0 || start > intervals[len(intervals)-1].end+1 {
			intervals = append(intervals, logInterval{
				start:       start,
				end:         end,
				tier:        tier,
				identifiers: append([]EvidenceIdentifier(nil), hit.identifiers...),
			})
			continue
		}

		last := &intervals[len(intervals)-1]
		if end > last.end {
			last.end = end
		}
		last.tier = strongerTier(last.tier, tier)
		last.identifiers = normalizeEvidenceIdentifiers(append(last.identifiers, hit.identifiers...))
	}

	return intervals
}

func readLogWindows(path, source string, intervals []logInterval, hits []logHit, remaining int) ([]LogWindow, error) {
	if remaining <= 0 {
		return nil, nil
	}

	reader, err := openLogReader(path)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	hitMap := make(map[int][]EvidenceIdentifier, len(hits))
	for _, hit := range hits {
		hitMap[hit.line] = hit.identifiers
	}

	scanner := newLogScanner(reader)
	var windows []LogWindow
	intervalIndex := 0
	lineNumber := 0

	for scanner.Scan() && intervalIndex < len(intervals) && remaining > 0 {
		lineNumber++
		for intervalIndex < len(intervals) && lineNumber > intervals[intervalIndex].end {
			intervalIndex++
		}
		if intervalIndex >= len(intervals) {
			break
		}

		interval := intervals[intervalIndex]
		if lineNumber < interval.start {
			continue
		}

		if len(windows) == 0 || windows[len(windows)-1].StartLine != interval.start || windows[len(windows)-1].Source != source {
			windows = append(windows, LogWindow{
				Source:             source,
				StartLine:          interval.start,
				EndLine:            interval.end,
				Tier:               interval.tier,
				MatchedIdentifiers: append([]EvidenceIdentifier(nil), interval.identifiers...),
			})
		}

		window := &windows[len(windows)-1]
		window.Lines = append(window.Lines, LogLine{
			Line:    lineNumber,
			Text:    scanner.Text(),
			Matches: append([]EvidenceIdentifier(nil), hitMap[lineNumber]...),
		})
		window.EndLine = lineNumber
		remaining--
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan failed: %w", err)
	}
	return windows, nil
}

func newLogScanner(reader io.Reader) *bufio.Scanner {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 10*1024*1024)
	return scanner
}

func matchedIdentifiers(line string, identifiers []EvidenceIdentifier) []EvidenceIdentifier {
	var matched []EvidenceIdentifier
	for _, identifier := range identifiers {
		if identifier.Value != "" && containsIdentifier(line, identifier.Value) {
			matched = append(matched, identifier)
		}
	}
	return normalizeEvidenceIdentifiers(matched)
}

func containsIdentifier(line, identifier string) bool {
	offset := 0
	for offset <= len(line)-len(identifier) {
		index := strings.Index(line[offset:], identifier)
		if index < 0 {
			return false
		}
		index += offset
		end := index + len(identifier)

		beforeBoundary := index == 0 || !isIdentifierByte(line[index-1])
		afterBoundary := end == len(line) || !isIdentifierByte(line[end])
		if beforeBoundary && afterBoundary {
			return true
		}
		offset = index + 1
	}
	return false
}

func isIdentifierByte(value byte) bool {
	return (value >= 'a' && value <= 'z') ||
		(value >= 'A' && value <= 'Z') ||
		(value >= '0' && value <= '9') ||
		value == '-' ||
		value == '_' ||
		value == '.'
}

func strongestIdentifierTier(identifiers []EvidenceIdentifier) EvidenceTier {
	tier := TierContextual
	for _, identifier := range identifiers {
		tier = strongerTier(tier, identifier.Tier)
	}
	return tier
}

type compoundReadCloser struct {
	io.Reader
	closers []io.Closer
}

func (reader *compoundReadCloser) Close() error {
	var firstErr error
	for _, closer := range reader.closers {
		if err := closer.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func openLogReader(path string) (io.ReadCloser, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	if !strings.HasSuffix(strings.ToLower(path), ".gz") {
		return file, nil
	}

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		file.Close()
		return nil, err
	}
	return &compoundReadCloser{
		Reader:  gzipReader,
		closers: []io.Closer{gzipReader, file},
	}, nil
}

func sortEvidenceIdentifiers(identifiers []EvidenceIdentifier) {
	sort.Slice(identifiers, func(i, j int) bool {
		if evidenceTierRank(identifiers[i].Tier) != evidenceTierRank(identifiers[j].Tier) {
			return evidenceTierRank(identifiers[i].Tier) < evidenceTierRank(identifiers[j].Tier)
		}
		if identifiers[i].Kind != identifiers[j].Kind {
			return identifiers[i].Kind < identifiers[j].Kind
		}
		return identifiers[i].Value < identifiers[j].Value
	})
}
