package chute

import (
	"fmt"
	"sort"
)

type CoverageReport struct {
	FilesDiscovered       int            `json:"files_discovered"`
	TopLevelFiles         int            `json:"top_level_files"`
	NodeArchiveFiles      int            `json:"node_archive_files"`
	YAMLFiles             int            `json:"yaml_files"`
	LogsAnalyzed          int            `json:"logs_analyzed"`
	RotatedLogsAnalyzed   int            `json:"rotated_logs_analyzed"`
	NodeArchivesFound     int            `json:"node_archives_found"`
	NodeArchivesExtracted int            `json:"node_archives_extracted"`
	ResourcesParsed       int            `json:"resources_parsed"`
	UnknownArtifacts      int            `json:"unknown_artifacts"`
	ParseFailures         int            `json:"parse_failures"`
	UnsupportedArtifacts  int            `json:"unsupported_artifacts"`
	Warnings              int            `json:"warnings"`
	CategoryCounts        map[string]int `json:"category_counts"`
}

func BuildCoverage(bundle *Bundle) CoverageReport {
	report := CoverageReport{
		FilesDiscovered: len(bundle.Inventory),
		ResourcesParsed: len(bundle.Resources),
		ParseFailures:   len(bundle.ParseErrors),
		CategoryCounts:  make(map[string]int),
	}

	for _, entry := range bundle.Inventory {
		report.CategoryCounts[entry.Category]++
		if entry.Origin == "node_archive" {
			report.NodeArchiveFiles++
		} else {
			report.TopLevelFiles++
		}

		switch entry.Category {
		case "yaml":
			report.YAMLFiles++
		case "log":
			report.LogsAnalyzed++
			if isRotatedLog(entry.Path) {
				report.RotatedLogsAnalyzed++
			}
		case "unknown":
			report.UnknownArtifacts++
		}
	}

	report.NodeArchivesFound = len(bundle.NodeArchives)
	for _, archive := range bundle.NodeArchives {
		if archive.Error == "" {
			report.NodeArchivesExtracted++
		}
	}

	report.UnsupportedArtifacts = report.UnknownArtifacts + (report.NodeArchivesFound - report.NodeArchivesExtracted)
	report.Warnings = len(AllWarnings(bundle))
	return report
}

func AllWarnings(bundle *Bundle) []BundleWarning {
	warnings := append([]BundleWarning(nil), bundle.Warnings...)
	for _, parseError := range bundle.ParseErrors {
		warnings = append(warnings, BundleWarning{
			Code:    "yaml_parse_failed",
			Source:  parseError.Source,
			Message: parseError.Error,
		})
	}

	unknown := 0
	for _, entry := range bundle.Inventory {
		if entry.Category == "unknown" {
			unknown++
		}
	}
	if unknown > 0 {
		warnings = append(warnings, BundleWarning{
			Code:    "unknown_artifacts",
			Message: fmt.Sprintf("%d artifacts were inventoried but not interpreted", unknown),
		})
	}

	sort.Slice(warnings, func(i, j int) bool {
		if warnings[i].Code != warnings[j].Code {
			return warnings[i].Code < warnings[j].Code
		}
		if warnings[i].Source != warnings[j].Source {
			return warnings[i].Source < warnings[j].Source
		}
		return warnings[i].Message < warnings[j].Message
	})
	return warnings
}
