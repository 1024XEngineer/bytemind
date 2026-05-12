package tui

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type FileType int

const (
	FileTypeImage FileType = iota
	FileTypeText
	FileTypePDF
	FileTypeBinary
	FileTypeUnknown

	maxFileReadSize = 100 * 1024 * 1024 // 100 MB
)

func classifyFile(path string) (FileType, string) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png":
		return FileTypeImage, "image/png"
	case ".jpg", ".jpeg":
		return FileTypeImage, "image/jpeg"
	case ".webp":
		return FileTypeImage, "image/webp"
	case ".gif":
		return FileTypeImage, "image/gif"
	case ".bmp":
		return FileTypeImage, "image/bmp"
	case ".txt", ".py", ".go", ".js", ".ts", ".java", ".c", ".cpp", ".cs",
		".rs", ".json", ".yaml", ".yml", ".md", ".csv", ".xml",
		".html", ".css", ".scss", ".less", ".sh", ".bash", ".bat", ".ps1",
		".toml", ".ini", ".cfg", ".conf", ".log", ".sql",
		".rb", ".php", ".swift", ".kt", ".kts", ".scala",
		".lua", ".r", ".m", ".mm", ".pl", ".pm",
		".hs", ".erl", ".ex", ".exs", ".clj", ".edn", ".dart",
		".proto", ".tf", ".hcl", ".cmake", ".make", ".mk",
		".gradle", ".sbt", ".dockerfile", ".gitignore", ".env":
		return FileTypeText, "text/plain"
	case ".pdf":
		return FileTypePDF, "application/pdf"
	default:
		return FileTypeUnknown, ""
	}
}

func resolvePath(token string) (string, error) {
	token = strings.TrimSpace(token)
	token = strings.Trim(token, `"'`)
	if token == "" {
		return "", fmt.Errorf("empty path")
	}

	if runtime.GOOS != "windows" {
		token = strings.ReplaceAll(token, `\ `, " ")
	}

	if strings.HasPrefix(token, "~") {
		home, err := os.UserHomeDir()
		if err == nil {
			token = filepath.Join(home, token[1:])
		}
	}

	if filepath.IsAbs(token) {
		return filepath.Clean(token), nil
	}

	if runtime.GOOS == "windows" && strings.HasPrefix(token, "/") {
		parts := strings.SplitN(token[1:], "/", 2)
		if len(parts) >= 1 && len(parts[0]) == 1 {
			c := parts[0][0]
			if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
				drive := strings.ToUpper(string(c)) + ":"
				rest := ""
				if len(parts) == 2 {
					rest = parts[1]
				}
				token = drive + "\\" + strings.ReplaceAll(rest, "/", "\\")
				return filepath.Clean(token), nil
			}
		}
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("cannot resolve relative path %q: %w", token, err)
	}
	return filepath.Clean(filepath.Join(cwd, token)), nil
}

func readFile(absPath string) (FileType, string, []byte, error) {
	info, err := os.Stat(absPath)
	if err != nil {
		return FileTypeUnknown, "", nil, fmt.Errorf("stat %s: %w", absPath, err)
	}
	if info.IsDir() {
		return FileTypeUnknown, "", nil, fmt.Errorf("%s is a directory", absPath)
	}
	if info.Size() > maxFileReadSize {
		return FileTypeUnknown, "", nil, fmt.Errorf("%s exceeds max file size (%d MB)", absPath, maxFileReadSize/(1024*1024))
	}

	fileType, mime := classifyFile(absPath)
	data, err := os.ReadFile(absPath)
	if err != nil {
		return FileTypeUnknown, "", nil, fmt.Errorf("read %s: %w", absPath, err)
	}

	switch fileType {
	case FileTypeImage:
		if strings.ToLower(filepath.Ext(absPath)) == ".bmp" {
			// TODO: BMP → PNG conversion
			return FileTypeImage, "image/png", data, nil
		}
		return FileTypeImage, mime, data, nil
	case FileTypeText:
		if bytes.Contains(data, []byte{0}) {
			return FileTypeBinary, "", nil, fmt.Errorf("%s contains null bytes", absPath)
		}
		return FileTypeText, mime, data, nil
	case FileTypePDF:
		// TODO: PDF text extraction
		return FileTypePDF, mime, data, nil
	case FileTypeUnknown:
		if bytes.Contains(data, []byte{0}) {
			return FileTypeBinary, "", nil, fmt.Errorf("%s is a binary file", absPath)
		}
		return FileTypeText, "text/plain", data, nil
	default:
		return fileType, mime, data, nil
	}
}

func extractFilePathsFromChunk(chunk string) []string {
	tokens := splitPathTokens(chunk)
	if len(tokens) == 0 {
		return nil
	}

	paths := make([]string, 0, len(tokens))
	candidateCount := 0
	for _, token := range tokens {
		token = strings.TrimSpace(strings.Trim(token, `"'`))
		if token == "" {
			continue
		}
		candidateCount++

		resolved, err := resolvePath(token)
		if err != nil {
			continue
		}
		info, err := os.Stat(resolved)
		if err != nil || info.IsDir() {
			continue
		}
		paths = append(paths, resolved)
	}

	if candidateCount == 0 || len(paths) != candidateCount {
		return nil
	}
	return paths
}
