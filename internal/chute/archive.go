package chute

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	if info.IsDir() {
		bundle, err := Load(absolute)
		return bundle, func() {}, err
	}

	tempDir, err := os.MkdirTemp("", "chute-*")
	if err != nil {
		return nil, func() {}, err
	}
	cleanup := func() { _ = os.RemoveAll(tempDir) }

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

	bundle, err := Load(root)
	if err != nil {
		cleanup()
		return nil, func() {}, err
	}
	bundle.Input = absolute
	return bundle, cleanup, nil
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
			// Ignore metadata entries and other non-file records.
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
