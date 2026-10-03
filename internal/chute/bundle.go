package chute

import (
	"fmt"
	"os"
	"path/filepath"
)

type Bundle struct {
	Root        string
	Inventory   []InventoryEntry
	Resources   []Resource
	ParseErrors []ParseError
	Index       *Index
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
		Root:        absolute,
		Inventory:   inventory,
		Resources:   resources,
		ParseErrors: parseErrors,
		Index:       NewIndex(resources),
	}, nil
}
