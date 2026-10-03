package chute

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func LoadInput(input string) (*Bundle, func(), error) {
	absolute, err := filepath.Abs(input)
	if err != nil {
		return nil, func() {}, err
	}

	info, err := os.Stat(absolute)
	if err != nil {
		return nil, func() {}, err
	}

	var bundle *Bundle
	var cleanupFns []func()
	cleanup := func() {
		for i := len(cleanupFns) - 1; i >= 0; i-- {
			cleanupFns[i]()
		}
	}

	if info.IsDir() {
		bundle, err = Load(absolute)
		if err != nil {
			return nil, func() {}, err
		}
	} else {
		tempDir, err := os.MkdirTemp("", "chute-*")
		if err != nil {
			return nil, func() {}, err
		}
		cleanupFns = append(cleanupFns, func() { _ = os.RemoveAll(tempDir) })

		switch {
		case strings.HasSuffix(strings.ToLower(absolute), ".zip"):
			err = extractZip(absolute, tempDir)
		case strings.HasSuffix(strings.ToLower(absolute), ".tar.gz"),
			strings.HasSuffix(strings.ToLower(absolute), ".tgz"):
			err = extractTarGz(absolute, tempDir)
		default:
			err = fmt.Errorf("unsupported input: %s (expected directory, .zip, .tar.gz, or .tgz)", absolute)
		}
		if err != nil {
			cleanup()
			return nil, func() {}, err
		}

		root, err := collapseSingleDirectory(tempDir)
		if err != nil {
			cleanup()
			return nil, func() {}, err
		}

		bundle, err = Load(root)
		if err != nil {
			cleanup()
			return nil, func() {}, err
		}
		bundle.Input = absolute
	}

	nodeTemp, err := os.MkdirTemp("", "chute-node-evidence-*")
	if err != nil {
		cleanup()
		return nil, func() {}, err
	}
	cleanupFns = append(cleanupFns, func() { _ = os.RemoveAll(nodeTemp) })

	ingestNodeArchives(bundle, nodeTemp)
	bundle.Index = NewIndex(bundle.Resources)

	return bundle, cleanup, nil
}

func ingestNodeArchives(bundle *Bundle, workspace string) {
	originalInventory := append([]InventoryEntry(nil), bundle.Inventory...)
	for _, entry := range originalInventory {
		if entry.Category != "node_archive" {
			continue
		}

		node := strings.TrimSuffix(filepath.Base(entry.Path), filepath.Ext(entry.Path))
		status := NodeArchiveStatus{Path: entry.Path, Node: node}
		destination := filepath.Join(workspace, SafeName(node))

		if err := os.MkdirAll(destination, 0o755); err != nil {
			status.Error = err.Error()
			bundle.NodeArchives = append(bundle.NodeArchives, status)
			bundle.Warnings = append(bundle.Warnings, BundleWarning{
				Code:    "node_archive_extract_failed",
				Source:  entry.Path,
				Message: err.Error(),
			})
			continue
		}

		if err := extractZip(physicalPath(bundle.Root, entry), destination); err != nil {
			status.Error = err.Error()
			bundle.NodeArchives = append(bundle.NodeArchives, status)
			bundle.Warnings = append(bundle.Warnings, BundleWarning{
				Code:    "node_archive_extract_failed",
				Source:  entry.Path,
				Message: err.Error(),
			})
			continue
		}

		extractedRoot, err := collapseSingleDirectory(destination)
		if err != nil {
			status.Error = err.Error()
			bundle.NodeArchives = append(bundle.NodeArchives, status)
			bundle.Warnings = append(bundle.Warnings, BundleWarning{
				Code:    "node_archive_inspect_failed",
				Source:  entry.Path,
				Message: err.Error(),
			})
			continue
		}

		nested, err := BuildInventory(extractedRoot)
		if err != nil {
			status.Error = err.Error()
			bundle.NodeArchives = append(bundle.NodeArchives, status)
			bundle.Warnings = append(bundle.Warnings, BundleWarning{
				Code:    "node_archive_inventory_failed",
				Source:  entry.Path,
				Message: err.Error(),
			})
			continue
		}

		for i := range nested {
			nested[i].Origin = "node_archive"
			nested[i].Node = node
			nested[i].Path = filepath.ToSlash(filepath.Join("nodes", node, "bundle", nested[i].Path))
			if nested[i].Category == "unknown" {
				nested[i].Category = "node_data"
			}
		}
		sort.Slice(nested, func(i, j int) bool { return nested[i].Path < nested[j].Path })

		resources, parseErrors := ParseResources(bundle.Root, nested)
		bundle.Resources = append(bundle.Resources, resources...)
		bundle.ParseErrors = append(bundle.ParseErrors, parseErrors...)
		bundle.Inventory = append(bundle.Inventory, nested...)

		status.ExtractedFiles = len(nested)
		bundle.NodeArchives = append(bundle.NodeArchives, status)
	}

	sort.Slice(bundle.Inventory, func(i, j int) bool {
		return bundle.Inventory[i].Path < bundle.Inventory[j].Path
	})
	sort.Slice(bundle.NodeArchives, func(i, j int) bool {
		return bundle.NodeArchives[i].Path < bundle.NodeArchives[j].Path
	})
}

func extractZip(path, destination string) error {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer reader.Close()

	for _, file := range reader.File {
		target, err := safeArchivePath(destination, file.Name)
		if err != nil {
			return err
		}

		if file.FileInfo().Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("archive contains unsupported symlink: %s", file.Name)
		}

		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}

		source, err := file.Open()
		if err != nil {
			return err
		}
		err = writeArchiveFile(target, source)
		closeErr := source.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	}

	return nil
}

func extractTarGz(path, destination string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzipReader.Close()

	reader := tar.NewReader(gzipReader)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		target, err := safeArchivePath(destination, header.Name)
		if err != nil {
			return err
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			if err := writeArchiveFile(target, reader); err != nil {
				return err
			}
		case tar.TypeSymlink, tar.TypeLink:
			return fmt.Errorf("archive contains unsupported link: %s", header.Name)
		default:
		}
	}

	return nil
}

func safeArchivePath(root, name string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(name))
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("unsafe archive path: %s", name)
	}

	target := filepath.Join(root, clean)
	relative, err := filepath.Rel(root, target)
	if err != nil {
		return "", err
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("unsafe archive path: %s", name)
	}
	return target, nil
}

func writeArchiveFile(path string, source io.Reader) error {
	target, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(target, source)
	closeErr := target.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func collapseSingleDirectory(root string) (string, error) {
	current := root
	for {
		entries, err := os.ReadDir(current)
		if err != nil {
			return "", err
		}
		if len(entries) != 1 || !entries[0].IsDir() {
			return current, nil
		}
		current = filepath.Join(current, entries[0].Name())
	}
}
