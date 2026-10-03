package chute

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type InventoryEntry struct {
	Path       string `json:"path"`
	Bytes      int64  `json:"bytes"`
	Category   string `json:"category"`
	Origin     string `json:"origin,omitempty"`
	Node       string `json:"node,omitempty"`
	SourcePath string `json:"-"`
}

var rotatedLogPattern = regexp.MustCompile(`(?i)\.log(?:\.\d+)?(?:\.gz)?$`)

func BuildInventory(root string) ([]InventoryEntry, error) {
	var entries []InventoryEntry

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}

		info, err := os.Stat(path)
		if err != nil {
			return err
		}

		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)

		entries = append(entries, InventoryEntry{
			Path:       relative,
			Bytes:      info.Size(),
			Category:   classifyPath(relative),
			Origin:     "bundle",
			SourcePath: path,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Path < entries[j].Path
	})

	return entries, nil
}

func classifyPath(path string) string {
	lower := strings.ToLower(filepath.ToSlash(path))
	normalized := "/" + strings.TrimPrefix(lower, "/")
	ext := strings.ToLower(filepath.Ext(lower))

	switch {
	case ext == ".yaml" || ext == ".yml":
		return "yaml"
	case strings.Contains(normalized, "/logs/") || rotatedLogPattern.MatchString(lower) || ext == ".txt":
		return "log"
	case strings.HasPrefix(normalized, "/nodes/") && ext == ".zip":
		return "node_archive"
	case strings.HasPrefix(normalized, "/nodes/"):
		return "node_data"
	default:
		return "unknown"
	}
}

func isRotatedLog(path string) bool {
	lower := strings.ToLower(path)
	return regexp.MustCompile(`\.log\.\d+(?:\.gz)?$`).MatchString(lower)
}

func physicalPath(root string, entry InventoryEntry) string {
	if entry.SourcePath != "" {
		return entry.SourcePath
	}
	return filepath.Join(root, filepath.FromSlash(entry.Path))
}
