package edit

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var ErrHashMismatch = errors.New("file changed since preview")

type Edit struct{}

type EditResult struct {
	Path       string
	OldHash    string
	NewHash    string
	OldContent string
	NewContent string
	Diff       string
}

func New() *Edit {
	return &Edit{}
}

func (e *Edit) Prepare(path, newContent string) (*EditResult, error) {
	oldContent, oldHash, err := readCurrent(path)
	if err != nil {
		return nil, err
	}

	newHash := hashString(newContent)
	return &EditResult{
		Path:       path,
		OldHash:    oldHash,
		NewHash:    newHash,
		OldContent: oldContent,
		NewContent: newContent,
		Diff:       e.GenerateDiff(path, oldContent, newContent),
	}, nil
}

func (e *Edit) Edit(path, newContent, expectedHash string) (*EditResult, error) {
	result, err := e.Prepare(path, newContent)
	if err != nil {
		return nil, err
	}

	if result.OldHash != expectedHash {
		return nil, ErrHashMismatch
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	tmpFile, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return nil, err
	}

	tmpPath := tmpFile.Name()
	success := false
	defer func() {
		_ = tmpFile.Close()
		if !success {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmpFile.WriteString(newContent); err != nil {
		return nil, err
	}

	if err := tmpFile.Chmod(0644); err != nil {
		return nil, err
	}

	if err := tmpFile.Close(); err != nil {
		return nil, err
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return nil, err
	}

	success = true
	return result, nil
}

func (e *Edit) GenerateDiff(path, oldContent, newContent string) string {
	oldLines := splitLines(oldContent)
	newLines := splitLines(newContent)

	var diff strings.Builder
	diff.WriteString(fmt.Sprintf("--- %s\n", path))
	diff.WriteString(fmt.Sprintf("+++ %s\n", path))

	maxLines := len(oldLines)
	if len(newLines) > maxLines {
		maxLines = len(newLines)
	}

	for i := 0; i < maxLines; i++ {
		switch {
		case i >= len(oldLines):
			diff.WriteString("+" + newLines[i] + "\n")
		case i >= len(newLines):
			diff.WriteString("-" + oldLines[i] + "\n")
		case oldLines[i] == newLines[i]:
			diff.WriteString(" " + oldLines[i] + "\n")
		default:
			diff.WriteString("-" + oldLines[i] + "\n")
			diff.WriteString("+" + newLines[i] + "\n")
		}
	}

	return diff.String()
}

func (e *Edit) ValidateHash(path, expectedHash string) (bool, error) {
	_, actualHash, err := readCurrent(path)
	if err != nil {
		return false, err
	}

	return actualHash == expectedHash, nil
}

func readCurrent(path string) (string, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", "", nil
		}
		return "", "", err
	}

	content := string(data)
	return content, hashBytes(data), nil
}

func hashString(content string) string {
	return hashBytes([]byte(content))
}

func hashBytes(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func splitLines(content string) []string {
	if content == "" {
		return []string{}
	}

	return strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
}
