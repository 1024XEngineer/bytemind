package edit

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
)

type Edit struct{}

func New() *Edit {
	return &Edit{}
}

type EditResult struct {
	Path       string
	OldHash    string
	NewHash    string
	OldContent string
	NewContent string
}

func (e *Edit) Edit(path, newContent string) (*EditResult, error) {
	oldContent := ""
	oldHash := ""

	if _, err := os.Stat(path); err == nil {
		data, _ := os.ReadFile(path)
		oldContent = string(data)
		hash := sha256.Sum256(data)
		oldHash = hex.EncodeToString(hash[:])
	}

	if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
		return nil, err
	}

	newHash := ""
	if data, err := os.ReadFile(path); err == nil {
		hash := sha256.Sum256(data)
		newHash = hex.EncodeToString(hash[:])
	}

	return &EditResult{
		Path:       path,
		OldHash:    oldHash,
		NewHash:    newHash,
		OldContent: oldContent,
		NewContent: newContent,
	}, nil
}

func (e *Edit) GenerateDiff(path, oldContent, newContent string) string {
	if oldContent == "" {
		return fmt.Sprintf("+++ %s\n+%s\n", path, newContent)
	}
	return fmt.Sprintf("--- %s\n+++ %s\n-%s\n+%s\n", path, path, oldContent, newContent)
}

func (e *Edit) ValidateHash(path, expectedHash string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}

	hash := sha256.Sum256(data)
	actualHash := hex.EncodeToString(hash[:])

	return actualHash == expectedHash, nil
}
