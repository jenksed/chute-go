package chute

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	DefaultLogContext = 5
	maxLogLines       = 20_000
)

type LogLine struct {
	Line int    `json:"line"`
	Text string `json:"text"`
}

type LogWindow struct {
	Source             string    `json:"source"`
	StartLine          int       `json:"start_line"`
	EndLine            int       `json:"end_line"`
	MatchedIdentifiers []string  `json:"matched_identifiers"`
	Lines              []LogLine `json:"lines"`
}

type LogReadError struct {
	Source string `json:"source"`
	Error  string `json:"error"`
}

type LogEvidence struct {
	Windows       []LogWindow    `json:"windows,omitempty"`
	MatchedLines  int            `json:"matched_lines"`
	IncludedLines int            `json:"included_lines"`
	ContextLines  int            `json:"context_lines"`
	Truncated     bool           `json:"truncated"`
	Errors        []LogReadError `json:"read_errors,omitempty"`
}

type logHit struct {
	line        int
	identifiers []string
}

type logInterval struct {
	start       int
	end         int
	identifiers []string
}

func ExtractLogs(root string, inventory []InventoryEntry, identifiers []string, contextLines int) LogEvidence {
	if contextLines < 0 {
		contextLines = 0
	}

	evidence := LogEvidence{ContextLines: contextLines}
	for _, entry := range inventory {
		if entry.Category != "log" {
			continue
		}
		scanLogFile(root, entry, identifiers, contextLines, &evidence)
	}

	evidence.Truncated = evidence.IncludedLines >= maxLogLines
	return evidence
}

func scanLogFile(root string, entry InventoryEntry, identifiers []string, contextLines int, evidence *LogEvidence) {
	path := filepath.Join(root, filepath.FromSlash(entry.Path))
	hits, err := findLogHits(path, identifiers)
	if err != nil {
		evidence.Errors = append(evidence.Errors, LogReadError{Source: entry.Path, Error: err.Error()})
		return
	}
	if len(hits) == 0 {
		return
	}

	evidence.MatchedLines += len(hits)
	intervals := mergeLogIntervals(hits, contextLines)

	windows, err := readLogWindows(path, entry.Path, intervals, maxLogLines-evidence.IncludedLines)
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

func findLogHits(path string, identifiers []string) ([]logHit, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := newLogScanner(file)
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

		if len(intervals) == 0 || start > intervals[len(intervals)-1].end+1 {
			intervals = append(intervals, logInterval{
				start:       start,
				end:         end,
				identifiers: append([]string(nil), hit.identifiers...),
			})
			continue
		}

		last := &intervals[len(intervals)-1]
		if end > last.end {
			last.end = end
		}
		last.identifiers = uniqueNonEmpty(append(last.identifiers, hit.identifiers...))
	}

	return intervals
}

func readLogWindows(path, source string, intervals []logInterval, remaining int) ([]LogWindow, error) {
	if remaining <= 0 {
		return nil, nil
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := newLogScanner(file)
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
				MatchedIdentifiers: append([]string(nil), interval.identifiers...),
			})
		}

		window := &windows[len(windows)-1]
		window.Lines = append(window.Lines, LogLine{Line: lineNumber, Text: scanner.Text()})
		window.EndLine = lineNumber
		remaining--
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan failed: %w", err)
	}
	return windows, nil
}

func newLogScanner(file *os.File) *bufio.Scanner {
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 10*1024*1024)
	return scanner
}

func matchedIdentifiers(line string, identifiers []string) []string {
	var matched []string
	for _, identifier := range identifiers {
		if identifier != "" && strings.Contains(line, identifier) {
			matched = append(matched, identifier)
		}
	}
	sort.Strings(matched)
	return uniqueNonEmpty(matched)
}
