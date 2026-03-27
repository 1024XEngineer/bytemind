package skilltool

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPackAndUnpackOfficeArchive(t *testing.T) {
	srcDir := filepath.Join(t.TempDir(), "src")
	if err := os.MkdirAll(filepath.Join(srcDir, "ppt", "slides"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "[Content_Types].xml"), []byte("content-types"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "ppt", "slides", "slide1.xml"), []byte("<slide>1</slide>"), 0o644); err != nil {
		t.Fatal(err)
	}

	archive := filepath.Join(t.TempDir(), "deck.pptx")
	if err := PackOfficeArchive(srcDir, archive); err != nil {
		t.Fatal(err)
	}

	outDir := filepath.Join(t.TempDir(), "out")
	if err := UnpackOfficeArchive(archive, outDir); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(outDir, "ppt", "slides", "slide1.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "<slide>1</slide>" {
		t.Fatalf("unexpected unpacked content: %q", string(data))
	}
}

func TestPackOfficeArchiveRejectsOutputInsideInputDir(t *testing.T) {
	srcDir := filepath.Join(t.TempDir(), "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "doc.xml"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := PackOfficeArchive(srcDir, filepath.Join(srcDir, "out.docx"))
	if err == nil {
		t.Fatal("expected output-inside-input error")
	}
}
