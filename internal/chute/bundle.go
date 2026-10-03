package chute

import (
	"fmt"
	"os"
	"path/filepath"
)

type BundleWarning struct {
	Code    string `json:"code"`
	Source  string `json:"source,omitempty"`
	Message string `json:"message"`
}

type NodeArchiveStatus struct {
	Path           string `json:"path"`
	Node           string `json:"node"`
	ExtractedFiles int    `json:"extracted_files"`
	Error          string `json:"error,omitempty"`
}

type Bundle struct {
	Input        string
	Root         string
	Inventory    []InventoryEntry
	Resources    []Resource
	ParseErrors  []ParseError
	Index        *Index
	Warnings     []BundleWarning
	NodeArchives []NodeArchiveStatus
}

func Load(root string) (*Bundle, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(absolute)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("not a directory: %s", absolute)
	}

	inventory, err := BuildInventory(absolute)
	if err != nil {
		return nil, err
	}

	resources, parseErrors := ParseResources(absolute, inventory)

	return &Bundle{
		Input:       absolute,
		Root:        absolute,
		Inventory:   inventory,
		Resources:   resources,
		ParseErrors: parseErrors,
		Index:       NewIndex(resources),
	}, nil
}
