package tui

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestClassifyFileImageExtensions(t *testing.T) {
	tests := []struct {
		ext      string
		fileType FileType
		mime     string
	}{
		{".png", FileTypeImage, "image/png"},
		{".jpg", FileTypeImage, "image/jpeg"},
		{".jpeg", FileTypeImage, "image/jpeg"},
		{".webp", FileTypeImage, "image/webp"},
		{".gif", FileTypeImage, "image/gif"},
		{".bmp", FileTypeImage, "image/bmp"},
	}
	for _, tc := range tests {
		ft, mime := classifyFile("test" + tc.ext)
		if ft != tc.fileType || mime != tc.mime {
			t.Errorf("classifyFile(%s) = (%v, %s), want (%v, %s)", tc.ext, ft, mime, tc.fileType, tc.mime)
		}
	}
}

func TestClassifyFileTextExtensions(t *testing.T) {
	textExts := []string{".txt", ".py", ".go", ".js", ".ts", ".java", ".c", ".cpp", ".cs",
		".rs", ".json", ".yaml", ".yml", ".md", ".csv", ".xml",
		".html", ".css", ".scss", ".sh", ".bash", ".bat", ".ps1",
		".toml", ".ini", ".cfg", ".conf", ".log", ".sql",
		".rb", ".php", ".swift", ".kt", ".kts", ".scala",
		".lua", ".r", ".hs", ".erl", ".ex", ".exs", ".clj", ".dart",
		".proto", ".tf", ".hcl", ".cmake", ".make", ".mk",
		".gradle", ".sbt", ".dockerfile", ".gitignore", ".env"}
	for _, ext := range textExts {
		ft, mime := classifyFile("file" + ext)
		if ft != FileTypeText {
			t.Errorf("classifyFile(%s) = %v, want FileTypeText", ext, ft)
		}
		if mime != "text/plain" {
			t.Errorf("classifyFile(%s) mime = %s, want text/plain", ext, mime)
		}
	}
}

func TestClassifyFilePDF(t *testing.T) {
	ft, mime := classifyFile("doc.pdf")
	if ft != FileTypePDF || mime != "application/pdf" {
		t.Errorf("classifyFile(.pdf) = (%v, %s), want (FileTypePDF, application/pdf)", ft, mime)
	}
}

func TestClassifyFileUnknown(t *testing.T) {
	ft, mime := classifyFile("data.bin")
	if ft != FileTypeUnknown || mime != "" {
		t.Errorf("classifyFile(.bin) = (%v, %s), want (FileTypeUnknown, \"\")", ft, mime)
	}
}

func TestClassifyFileCaseInsensitive(t *testing.T) {
	ft, _ := classifyFile("image.PNG")
	if ft != FileTypeImage {
		t.Errorf("classifyFile(.PNG) = %v, want FileTypeImage", ft)
	}
	ft, _ = classifyFile("script.PY")
	if ft != FileTypeText {
		t.Errorf("classifyFile(.PY) = %v, want FileTypeText", ft)
	}
}

func TestResolvePathAbsolute(t *testing.T) {
	absPath := filepath.Join(t.TempDir(), "test.png")
	os.WriteFile(absPath, []byte("png"), 0o644)

	resolved, err := resolvePath(absPath)
	if err != nil {
		t.Fatalf("resolvePath(%s): %v", absPath, err)
	}
	if resolved != filepath.Clean(absPath) {
		t.Errorf("resolvePath = %s, want %s", resolved, filepath.Clean(absPath))
	}
}

func TestResolvePathRelative(t *testing.T) {
	cwd, _ := os.Getwd()
	resolved, err := resolvePath("relative/path/file.txt")
	if err != nil {
		t.Fatalf("resolvePath: %v", err)
	}
	expected := filepath.Clean(filepath.Join(cwd, "relative/path/file.txt"))
	if resolved != expected {
		t.Errorf("resolvePath = %s, want %s", resolved, expected)
	}
}

func TestResolvePathHomeExpansion(t *testing.T) {
	resolved, err := resolvePath("~/test.txt")
	if err != nil {
		t.Fatalf("resolvePath(~): %v", err)
	}
	home, _ := os.UserHomeDir()
	expected := filepath.Clean(filepath.Join(home, "test.txt"))
	if resolved != expected {
		t.Errorf("resolvePath(~) = %s, want %s", resolved, expected)
	}
}

func TestResolvePathEmptyString(t *testing.T) {
	_, err := resolvePath("")
	if err == nil {
		t.Error("resolvePath(\"\") should return error")
	}
}

func TestResolvePathTrimQuotes(t *testing.T) {
	cwd, _ := os.Getwd()
	resolved, err := resolvePath(`"test.txt"`)
	if err != nil {
		t.Fatalf("resolvePath: %v", err)
	}
	if !strings.Contains(resolved, "test.txt") {
		t.Errorf("resolvePath with quotes = %s", resolved)
	}
	// verify no leftover quotes
	if strings.Contains(resolved, `"`) {
		t.Errorf("resolved path still has quotes: %s", resolved)
	}
	_ = cwd
}

func TestResolvePathWindowsPOSIX(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("POSIX path conversion only applies on Windows")
	}
	resolved, err := resolvePath("/c/Users/test/file.txt")
	if err != nil {
		t.Fatalf("resolvePath: %v", err)
	}
	if !strings.HasPrefix(resolved, "C:\\") {
		t.Errorf("expected Windows drive prefix, got: %s", resolved)
	}
}

func TestReadFileImageType(t *testing.T) {
	dir := t.TempDir()
	imagePath := filepath.Join(dir, "test.png")
	os.WriteFile(imagePath, []byte("fake-png-data"), 0o644)

	ft, mime, data, err := readFile(imagePath)
	if err != nil {
		t.Fatalf("readFile: %v", err)
	}
	if ft != FileTypeImage {
		t.Errorf("file type = %v, want FileTypeImage", ft)
	}
	if mime != "image/png" {
		t.Errorf("mime = %s, want image/png", mime)
	}
	if string(data) != "fake-png-data" {
		t.Errorf("data = %s, want fake-png-data", string(data))
	}
}

func TestReadFileTextType(t *testing.T) {
	dir := t.TempDir()
	textPath := filepath.Join(dir, "readme.txt")
	os.WriteFile(textPath, []byte("hello world"), 0o644)

	ft, _, data, err := readFile(textPath)
	if err != nil {
		t.Fatalf("readFile: %v", err)
	}
	if ft != FileTypeText {
		t.Errorf("file type = %v, want FileTypeText", ft)
	}
	if string(data) != "hello world" {
		t.Errorf("data = %s, want hello world", string(data))
	}
}

func TestReadFileDirectory(t *testing.T) {
	dir := t.TempDir()
	_, _, _, err := readFile(dir)
	if err == nil {
		t.Error("readFile on directory should return error")
	}
	if !strings.Contains(err.Error(), "directory") {
		t.Errorf("error should mention directory, got: %v", err)
	}
}

func TestReadFileNotFound(t *testing.T) {
	_, _, _, err := readFile("/nonexistent/file.txt")
	if err == nil {
		t.Error("readFile on nonexistent file should return error")
	}
}

func TestReadFileUnknownExtensionTreatAsText(t *testing.T) {
	dir := t.TempDir()
	unknownPath := filepath.Join(dir, "data.unknown")
	os.WriteFile(unknownPath, []byte("readable content"), 0o644)

	ft, mime, data, err := readFile(unknownPath)
	if err != nil {
		t.Fatalf("readFile: %v", err)
	}
	if ft != FileTypeText {
		t.Errorf("unknown ext with text content: file type = %v, want FileTypeText", ft)
	}
	if mime != "text/plain" {
		t.Errorf("mime = %s, want text/plain", mime)
	}
	if string(data) != "readable content" {
		t.Errorf("data mismatch")
	}
}

func TestReadFileBinaryDetection(t *testing.T) {
	dir := t.TempDir()
	binPath := filepath.Join(dir, "binary.unknown")
	os.WriteFile(binPath, []byte{0x00, 0x01, 0x02, 0x00}, 0o644)

	ft, _, _, err := readFile(binPath)
	if err == nil {
		t.Error("readFile on binary file should return error")
	}
	if ft != FileTypeBinary {
		t.Errorf("file type = %v, want FileTypeBinary", ft)
	}
	if !strings.Contains(err.Error(), "binary") {
		t.Errorf("error should mention binary, got: %v", err)
	}
}

func TestExtractFilePathsFromChunkAbsolute(t *testing.T) {
	dir := t.TempDir()
	img := filepath.Join(dir, "photo.png")
	txt := filepath.Join(dir, "notes.txt")
	os.WriteFile(img, []byte("png"), 0o644)
	os.WriteFile(txt, []byte("txt"), 0o644)

	paths := extractFilePathsFromChunk(img + " " + txt)
	if len(paths) != 2 {
		t.Fatalf("expected 2 paths, got %d: %v", len(paths), paths)
	}
	if paths[0] != filepath.Clean(img) || paths[1] != filepath.Clean(txt) {
		t.Errorf("unexpected paths: %v", paths)
	}
}

func TestExtractFilePathsFromChunkEmpty(t *testing.T) {
	paths := extractFilePathsFromChunk("")
	if paths != nil {
		t.Errorf("expected nil for empty chunk, got %v", paths)
	}
}

func TestExtractFilePathsFromChunkNoFiles(t *testing.T) {
	paths := extractFilePathsFromChunk("not a real file path anywhere")
	if paths != nil {
		t.Errorf("expected nil, got %v", paths)
	}
}

func TestExtractFilePathsFromChunkQuotedPaths(t *testing.T) {
	dir := t.TempDir()
	pathWithSpaces := filepath.Join(dir, "my photos", "vacation.png")
	os.MkdirAll(filepath.Dir(pathWithSpaces), 0o755)
	os.WriteFile(pathWithSpaces, []byte("png"), 0o644)

	paths := extractFilePathsFromChunk(`"` + pathWithSpaces + `"`)
	if len(paths) != 1 {
		t.Fatalf("expected 1 path for quoted path, got %d: %v", len(paths), paths)
	}
}
