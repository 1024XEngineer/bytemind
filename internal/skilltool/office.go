package skilltool

import (
	"archive/zip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func UnpackOfficeArchive(input, outputDir string) error {
	reader, err := zip.OpenReader(input)
	if err != nil {
		return err
	}
	defer reader.Close()

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return err
	}

	base, err := filepath.Abs(outputDir)
	if err != nil {
		return err
	}

	for _, file := range reader.File {
		target, err := safeJoin(base, file.Name)
		if err != nil {
			return err
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

		src, err := file.Open()
		if err != nil {
			return err
		}

		dst, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, file.Mode())
		if err != nil {
			src.Close()
			return err
		}

		_, copyErr := io.Copy(dst, src)
		closeErr := dst.Close()
		srcErr := src.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if srcErr != nil {
			return srcErr
		}
	}

	return nil
}

func PackOfficeArchive(inputDir, output string) error {
	inputAbs, err := filepath.Abs(inputDir)
	if err != nil {
		return err
	}
	outputAbs, err := filepath.Abs(output)
	if err != nil {
		return err
	}

	relToInput, err := filepath.Rel(inputAbs, outputAbs)
	if err == nil && relToInput != "." && !strings.HasPrefix(relToInput, "..") {
		return fmt.Errorf("output file must not be inside input directory: %s", output)
	}

	if err := os.MkdirAll(filepath.Dir(outputAbs), 0o755); err != nil {
		return err
	}

	dst, err := os.Create(outputAbs)
	if err != nil {
		return err
	}
	defer dst.Close()

	writer := zip.NewWriter(dst)

	err = filepath.WalkDir(inputAbs, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(inputAbs, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

		info, err := entry.Info()
		if err != nil {
			return err
		}

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = rel
		header.Method = zip.Deflate

		w, err := writer.CreateHeader(header)
		if err != nil {
			return err
		}

		src, err := os.Open(path)
		if err != nil {
			return err
		}

		_, copyErr := io.Copy(w, src)
		closeErr := src.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	if err != nil {
		_ = writer.Close()
		return err
	}

	return writer.Close()
}

func safeJoin(base, rel string) (string, error) {
	target := filepath.Join(base, filepath.FromSlash(rel))
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	if targetAbs != base && !strings.HasPrefix(targetAbs, base+string(os.PathSeparator)) {
		return "", fmt.Errorf("archive entry escapes target directory: %s", rel)
	}
	return targetAbs, nil
}
