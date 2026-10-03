package chute

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

type TimelineEntry struct {
	Timestamp string       `json:"timestamp"`
	Source    string       `json:"source"`
	Line      int          `json:"line,omitempty"`
	Type      string       `json:"type"`
	Tier      EvidenceTier `json:"tier"`
	Summary   string       `json:"summary"`
}

var logTimestampPattern = regexp.MustCompile(`\b\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})\b`)

func FindRelatedEvents(index *Index, identifiers []EvidenceIdentifier) []Resource {
	var events []Resource
	seen := make(map[string]struct{})

	for _, event := range index.Kind("Event") {
		if _, ok := eventEvidenceTier(event, identifiers); !ok {
			continue
		}
		key := event.UID
		if key == "" {
			key = event.APIVersion + "|" + event.Namespace + "|" + event.Name
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		events = append(events, event)
	}

	sort.Slice(events, func(i, j int) bool {
		left, _ := eventTime(events[i])
		right, _ := eventTime(events[j])
		if left.Equal(right) {
			return events[i].Source < events[j].Source
		}
		return left.Before(right)
	})
	return events
}

func BuildTimeline(events []Resource, logs LogEvidence, identifiers []EvidenceIdentifier) []TimelineEntry {
	var entries []TimelineEntry

	for _, event := range events {
		timestamp, ok := eventTime(event)
		if !ok {
			continue
		}
		tier, related := eventEvidenceTier(event, identifiers)
		if !related {
			continue
		}
		entries = append(entries, TimelineEntry{
			Timestamp: timestamp.UTC().Format(time.RFC3339Nano),
			Source:    event.Source,
			Type:      "kubernetes_event",
			Tier:      tier,
			Summary:   eventSummary(event),
		})
	}

	for _, window := range logs.Windows {
		for _, line := range window.Lines {
			if len(line.Matches) == 0 {
				continue
			}
			timestamp, ok := timestampFromLogLine(line.Text)
			if !ok {
				continue
			}
			entries = append(entries, TimelineEntry{
				Timestamp: timestamp.UTC().Format(time.RFC3339Nano),
				Source:    window.Source,
				Line:      line.Line,
				Type:      "log",
				Tier:      strongestIdentifierTier(line.Matches),
				Summary:   strings.TrimSpace(line.Text),
			})
		}
	}

	seen := make(map[string]struct{})
	filtered := make([]TimelineEntry, 0, len(entries))
	for _, entry := range entries {
		key := entry.Timestamp + "|" + entry.Type + "|" + entry.Source + "|" + fmt.Sprint(entry.Line) + "|" + entry.Summary
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		filtered = append(filtered, entry)
	}

	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].Timestamp != filtered[j].Timestamp {
			return filtered[i].Timestamp < filtered[j].Timestamp
		}
		if evidenceTierRank(filtered[i].Tier) != evidenceTierRank(filtered[j].Tier) {
			return evidenceTierRank(filtered[i].Tier) < evidenceTierRank(filtered[j].Tier)
		}
		if filtered[i].Source != filtered[j].Source {
			return filtered[i].Source < filtered[j].Source
		}
		return filtered[i].Line < filtered[j].Line
	})
	return filtered
}

func eventEvidenceTier(event Resource, identifiers []EvidenceIdentifier) (EvidenceTier, bool) {
	values := []string{
		nestedString(event.Data, "involvedObject", "name"),
		nestedString(event.Data, "involvedObject", "uid"),
		nestedString(event.Data, "regarding", "name"),
		nestedString(event.Data, "regarding", "uid"),
		eventMessage(event),
	}

	found := false
	tier := TierContextual
	for _, identifier := range identifiers {
		for _, value := range values {
			if value != "" && strings.Contains(value, identifier.Value) {
				if !found {
					tier = identifier.Tier
					found = true
				} else {
					tier = strongerTier(tier, identifier.Tier)
				}
			}
		}
	}
	return tier, found
}

func eventSummary(event Resource) string {
	eventType := nestedString(event.Data, "type")
	reason := nestedString(event.Data, "reason")
	message := eventMessage(event)

	parts := make([]string, 0, 3)
	if eventType != "" {
		parts = append(parts, eventType)
	}
	if reason != "" {
		parts = append(parts, reason)
	}
	if message != "" {
		parts = append(parts, message)
	}
	if len(parts) == 0 {
		return event.Kind + " " + event.Name
	}
	return strings.Join(parts, ": ")
}

func eventMessage(event Resource) string {
	if note := nestedString(event.Data, "note"); note != "" {
		return note
	}
	return nestedString(event.Data, "message")
}

func eventTime(event Resource) (time.Time, bool) {
	paths := [][]string{
		{"eventTime"},
		{"series", "lastObservedTime"},
		{"lastTimestamp"},
		{"firstTimestamp"},
		{"metadata", "creationTimestamp"},
	}
	for _, path := range paths {
		if value := nestedValue(event.Data, path...); value != nil {
			if timestamp, ok := parseTimeValue(value); ok {
				return timestamp, true
			}
		}
	}
	return time.Time{}, false
}

func parseTimeValue(value any) (time.Time, bool) {
	switch typed := value.(type) {
	case time.Time:
		return typed, true
	case string:
		layouts := []string{
			time.RFC3339Nano,
			time.RFC3339,
			"2006-01-02 15:04:05 -0700 MST",
		}
		for _, layout := range layouts {
			if timestamp, err := time.Parse(layout, typed); err == nil {
				return timestamp, true
			}
		}
	}
	return time.Time{}, false
}

func timestampFromLogLine(line string) (time.Time, bool) {
	value := logTimestampPattern.FindString(line)
	if value == "" {
		return time.Time{}, false
	}
	timestamp, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, false
	}
	return timestamp, true
}
