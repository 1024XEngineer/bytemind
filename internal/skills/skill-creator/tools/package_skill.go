package main

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var (
	excludeDirs     = map[string]struct{}{"node_modules": {}}
	excludeFiles    = map[string]struct{}{".DS_Store": {}}
	excludeGlobs    = []string{}
	rootExcludeDirs = map[string]struct{}{"evals": {}}
)

func runPackageSkill(args []string) error {
	fs, err := parseFlagSet("package-skill", args)
	if err != nil {
		return err
	}
	if fs.NArg() < 1 || fs.NArg() > 2 {
		return fmt.Errorf("usage: go run ./internal/skills/skill-creator/tools package-skill <path/to/skill-folder> [output-directory]")
	}

	skillPath := fs.Arg(0)
	outputDir := ""
	if fs.NArg() == 2 {
		outputDir = fs.Arg(1)
	}

	fmt.Printf("Packaging skill: %s\n", skillPath)
	if outputDir != "" {
		fmt.Printf("Output directory: %s\n", outputDir)
	}

	out, err := packageSkill(skillPath, outputDir)
	if err != nil {
		return err
	}
	fmt.Printf("Successfully packaged skill to: %s\n", out)
	return nil
}

func packageSkill(skillPath string, outputDir string) (string, error) {
	skillAbs := mustAbs(strings.TrimSpace(skillPath))
	info, err := os.Stat(skillAbs)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("skill folder not found: %s", skillAbs)
		}
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("path is not a directory: %s", skillAbs)
	}
	if _, err := os.Stat(filepath.Join(skillAbs, "SKILL.md")); err != nil {
		return "", fmt.Errorf("SKILL.md not found in %s", skillAbs)
	}

	fmt.Println("Validating skill...")
	v := validateSkill(skillAbs)
	if !v.Valid {
		return "", fmt.Errorf("validation failed: %s", v.Message)
	}
	fmt.Println(v.Message)

	outDir := ""
	if strings.TrimSpace(outputDir) != "" {
		outDir = mustAbs(outputDir)
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			return "", err
		}
	} else {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		outDir = cwd
	}

	skillName := filepath.Base(skillAbs)
	outPath := filepath.Join(outDir, skillName+".skill")

	file, err := os.Create(outPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	zipWriter := zip.NewWriter(file)
	defer zipWriter.Close()

	skillParent := filepath.Dir(skillAbs)
	err = filepath.WalkDir(skillAbs, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(skillParent, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		if shouldExcludePath(rel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			fmt.Printf("Skipped: %s\n", rel)
			return nil
		}
		if d.IsDir() {
			return nil
		}

		src, err := os.Open(path)
		if err != nil {
			return err
		}

		info, err := d.Info()
		if err != nil {
			_ = src.Close()
			return err
		}
		hdr, err := zip.FileInfoHeader(info)
		if err != nil {
			_ = src.Close()
			return err
		}
		hdr.Name = rel
		hdr.Method = zip.Deflate

		w, err := zipWriter.CreateHeader(hdr)
		if err != nil {
			_ = src.Close()
			return err
		}
		if _, err := io.Copy(w, src); err != nil {
			_ = src.Close()
			return err
		}
		_ = src.Close()
		fmt.Printf("Added: %s\n", rel)
		return nil
	})
	if err != nil {
		return "", err
	}

	return outPath, nil
}

func shouldExcludePath(relPath string) bool {
	parts := strings.Split(filepath.ToSlash(relPath), "/")
	for _, p := range parts {
		if _, ok := excludeDirs[p]; ok {
			return true
		}
	}
	if len(parts) > 1 {
		if _, ok := rootExcludeDirs[parts[1]]; ok {
			return true
		}
	}
	name := parts[len(parts)-1]
	if _, ok := excludeFiles[name]; ok {
		return true
	}
	for _, pattern := range excludeGlobs {
		if ok, _ := filepath.Match(pattern, name); ok {
			return true
		}
	}
	return false
}
