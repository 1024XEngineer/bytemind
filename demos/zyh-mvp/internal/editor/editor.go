package editor

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
)

var ErrConflict = errors.New("file changed after it was read")

type Service struct{}

func (s Service) Preview(oldContent, newContent []byte) string {
	return unifiedLineDiff(string(oldContent), string(newContent))
}

func (s Service) Apply(path, expectedHash string, newContent []byte) error {
	current, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read current file before write: %w", err)
	}

	if hashBytes(current) != expectedHash {
		return ErrConflict
	}

	tempPath := path + ".forgecli.tmp"
	if err := os.WriteFile(tempPath, newContent, 0o644); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}

	if err := os.Rename(tempPath, path); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("replace target file: %w", err)
	}

	return nil
}

func unifiedLineDiff(oldValue, newValue string) string {
	oldLines := splitLines(oldValue)
	newLines := splitLines(newValue)
	dp := lcsTable(oldLines, newLines)

	var lines []string
	lines = append(lines, "--- old")
	lines = append(lines, "+++ new")
	lines = append(lines, buildDiffLines(oldLines, newLines, dp, 0, 0)...)
	return strings.Join(lines, "\n")
}

func buildDiffLines(oldLines, newLines []string, dp [][]int, i, j int) []string {
	if i >= len(oldLines) && j >= len(newLines) {
		return nil
	}
	if i >= len(oldLines) {
		result := make([]string, 0, len(newLines)-j)
		for ; j < len(newLines); j++ {
			result = append(result, "+ "+strings.TrimRight(newLines[j], "\n"))
		}
		return result
	}
	if j >= len(newLines) {
		result := make([]string, 0, len(oldLines)-i)
		for ; i < len(oldLines); i++ {
			result = append(result, "- "+strings.TrimRight(oldLines[i], "\n"))
		}
		return result
	}
	if oldLines[i] == newLines[j] {
		return append([]string{"  " + strings.TrimRight(oldLines[i], "\n")}, buildDiffLines(oldLines, newLines, dp, i+1, j+1)...)
	}
	if dp[i+1][j] >= dp[i][j+1] {
		return append([]string{"- " + strings.TrimRight(oldLines[i], "\n")}, buildDiffLines(oldLines, newLines, dp, i+1, j)...)
	}
	return append([]string{"+ " + strings.TrimRight(newLines[j], "\n")}, buildDiffLines(oldLines, newLines, dp, i, j+1)...)
}

func lcsTable(oldLines, newLines []string) [][]int {
	dp := make([][]int, len(oldLines)+1)
	for i := range dp {
		dp[i] = make([]int, len(newLines)+1)
	}

	for i := len(oldLines) - 1; i >= 0; i-- {
		for j := len(newLines) - 1; j >= 0; j-- {
			if oldLines[i] == newLines[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}

	return dp
}

func splitLines(value string) []string {
	if value == "" {
		return []string{}
	}
	return strings.SplitAfter(value, "\n")
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
