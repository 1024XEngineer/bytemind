package app

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func pathContainsDir(pathEnv, targetDir string) bool {
	return pathContainsDirForOS(pathEnv, targetDir, runtime.GOOS == "windows")
}

func appendPathEntry(pathEnv, targetDir string, windows bool) (string, bool) {
	target := strings.TrimSpace(strings.Trim(targetDir, `"`))
	if target == "" {
		return pathEnv, false
	}
	if pathContainsDirForOS(pathEnv, target, windows) {
		return pathEnv, false
	}
	if strings.TrimSpace(pathEnv) == "" {
		return target, true
	}
	sep := string(os.PathListSeparator)
	if windows {
		sep = ";"
	}
	cleanBase := strings.TrimRight(pathEnv, sep+" ")
	if cleanBase == "" {
		return target, true
	}
	return cleanBase + sep + target, true
}

func pathContainsDirForOS(pathEnv, targetDir string, windows bool) bool {
	target := normalizePathEntry(targetDir, windows)
	if target == "" {
		return false
	}
	for _, item := range splitPathListForOS(pathEnv, windows) {
		if normalizePathEntry(item, windows) == target {
			return true
		}
	}
	return false
}

func splitPathListForOS(pathEnv string, windows bool) []string {
	if windows {
		return strings.Split(pathEnv, ";")
	}
	return filepath.SplitList(pathEnv)
}

func normalizePathEntry(value string, windows bool) string {
	value = strings.TrimSpace(strings.Trim(value, `"`))
	if value == "" {
		return ""
	}
	if windows {
		value = strings.ReplaceAll(value, `\`, `/`)
		value = filepath.Clean(value)
		return strings.ToLower(value)
	}
	return filepath.Clean(value)
}
