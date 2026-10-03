package chute

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type InventoryEntry struct {
	Path     string `json:"path"`
	Bytes    int64  `json:"bytes"`
	Category string `json:"category"`
}

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
			Path:     relative,
			Bytes:    info.Size(),
			Category: classifyPath(relative),
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
	ext := strings.ToLower(filepath.Ext(lower))

	switch {
	case ext == ".yaml" || ext == ".yml":
		return "yaml"
	case strings.Contains(lower, "/logs/") || ext == ".log" || ext == ".txt":
		return "log"
	case strings.Contains(lower, "/nodes/"):
		return "node_data"
	default:
		return "unknown"
	}
}
