package chute

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const maxLogMatches = 20_000

type LogMatch struct {
	Source string `json:"source"`
	Line   int    `json:"line"`
	Text   string `json:"text"`
}

type LogReadError struct {
	Source string `json:"source"`
	Error  string `json:"error"`
}

type LogEvidence struct {
	Matches       []LogMatch     `json:"matches,omitempty"`
	MatchedLines  int            `json:"matched_lines"`
	IncludedLines int            `json:"included_lines"`
	Truncated     bool           `json:"truncated"`
	Errors        []LogReadError `json:"read_errors,omitempty"`
}

func ExtractLogs(root string, inventory []InventoryEntry, identifiers []string) LogEvidence {
	var evidence LogEvidence

	for _, entry := range inventory {
		if entry.Category != "log" {
			continue
		}
		scanLogFile(root, entry, identifiers, &evidence)
	}

	evidence.Truncated = evidence.MatchedLines > evidence.IncludedLines
	return evidence
}

func scanLogFile(root string, entry InventoryEntry, identifiers []string, evidence *LogEvidence) {
	path := filepath.Join(root, filepath.FromSlash(entry.Path))
	file, err := os.Open(path)
	if err != nil {
		evidence.Errors = append(evidence.Errors, LogReadError{Source: entry.Path, Error: err.Error()})
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 10*1024*1024)

	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()
		if !lineMatches(line, identifiers) {
			continue
		}

		evidence.MatchedLines++
		if evidence.IncludedLines >= maxLogMatches {
			continue
		}

		evidence.Matches = append(evidence.Matches, LogMatch{
			Source: entry.Path,
			Line:   lineNumber,
			Text:   line,
		})
		evidence.IncludedLines++
	}

	if err := scanner.Err(); err != nil {
		evidence.Errors = append(evidence.Errors, LogReadError{
			Source: entry.Path,
			Error:  fmt.Sprintf("scan failed: %v", err),
		})
	}
}

func lineMatches(line string, identifiers []string) bool {
	for _, identifier := range identifiers {
		if identifier != "" && strings.Contains(line, identifier) {
			return true
		}
	}
	return false
}
