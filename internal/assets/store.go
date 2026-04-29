package assets

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	corepkg "github.com/1024XEngineer/bytemind/internal/core"
	"github.com/1024XEngineer/bytemind/internal/llm"
)

type PutImageInput struct {
	SessionID corepkg.SessionID
	ImageID   int
	MediaType string
	FileName  string
	Data      []byte
	Width     int
	Height    int
}

type ImageMeta struct {
	AssetID   llm.AssetID
	ImageID   int
	MediaType string
	FileName  string
	CachePath string
	ByteSize  int64
	Width     int
	Height    int
}

type ImageBlob struct {
	SessionID corepkg.SessionID
	ImageID   int
	MediaType string
	FileName  string
	Data      []byte
	ByteSize  int64
	CachePath string
}

type ImageStore interface {
	PutImage(ctx context.Context, in PutImageInput) (ImageMeta, error)
	GetImageByAssetID(ctx context.Context, sessionID corepkg.SessionID, assetID llm.AssetID) (ImageBlob, error)
	DeleteSessionImages(ctx context.Context, sessionID corepkg.SessionID) error
	GC(ctx context.Context, keepSessionIDs []corepkg.SessionID, olderThan time.Time) error
}

type FileAssetStore struct {
	root string
}

func NewFileAssetStore(configDir string) (*FileAssetStore, error) {
	base := strings.TrimSpace(configDir)
	if base == "" {
		return nil, fmt.Errorf("config directory is required")
	}
	root := filepath.Join(base, "image-cache")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	return &FileAssetStore{root: root}, nil
}

func (s *FileAssetStore) PutImage(ctx context.Context, in PutImageInput) (ImageMeta, error) {
	if err := ctx.Err(); err != nil {
		return ImageMeta{}, err
	}
	sessionID := sanitizeSessionID(string(in.SessionID))
	if sessionID == "" {
		return ImageMeta{}, llm.WrapError("asset_store", llm.ErrorCodeUnknown, fmt.Errorf("session id is required"))
	}
	if in.ImageID < 0 {
		return ImageMeta{}, llm.WrapError("asset_store", llm.ErrorCodeUnknown, fmt.Errorf("image id must be >= 0"))
	}
	ext, ok := extensionForMediaType(strings.TrimSpace(strings.ToLower(in.MediaType)))
	if !ok {
		return ImageMeta{}, llm.WrapError("asset_store", llm.ErrorCodeUnsupportedImage, fmt.Errorf("unsupported media type %q", in.MediaType))
	}
	if len(in.Data) == 0 {
		return ImageMeta{}, llm.WrapError("asset_store", llm.ErrorCodeImageDecodeFailed, fmt.Errorf("image payload is empty"))
	}

	sessionDir := filepath.Join(s.root, sessionID)
	target := filepath.Join(sessionDir, strconv.Itoa(in.ImageID)+"."+ext)
	if err := ensureWithinRoot(s.root, target); err != nil {
		return ImageMeta{}, err
	}
	if err := writeAtomicFile(target, in.Data); err != nil {
		return ImageMeta{}, err
	}

	rel, err := filepath.Rel(s.root, target)
	if err != nil {
		return ImageMeta{}, err
	}
	return ImageMeta{
		AssetID:   AssetID(in.SessionID, in.ImageID),
		ImageID:   in.ImageID,
		MediaType: strings.ToLower(in.MediaType),
		FileName:  in.FileName,
		CachePath: filepath.ToSlash(rel),
		ByteSize:  int64(len(in.Data)),
		Width:     in.Width,
		Height:    in.Height,
	}, nil
}

func (s *FileAssetStore) GetImageByAssetID(ctx context.Context, sessionID corepkg.SessionID, assetID llm.AssetID) (ImageBlob, error) {
	if err := ctx.Err(); err != nil {
		return ImageBlob{}, err
	}
	assetSessionID, imageID, err := parseAssetID(assetID)
	if err != nil {
		return ImageBlob{}, llm.WrapError("asset_store", llm.ErrorCodeAssetNotFound, err)
	}
	sanitized := sanitizeSessionID(string(sessionID))
	if sanitized == "" {
		return ImageBlob{}, llm.WrapError("asset_store", llm.ErrorCodeUnknown, fmt.Errorf("session id is required"))
	}
	if sanitizeSessionID(string(assetSessionID)) != sanitized {
		return ImageBlob{}, llm.WrapError("asset_store", llm.ErrorCodeAssetNotFound, fmt.Errorf("asset %q does not belong to session %q", assetID, sessionID))
	}
	sessionDir := filepath.Join(s.root, sanitized)
	if err := ensureWithinRoot(s.root, sessionDir); err != nil {
		return ImageBlob{}, err
	}

	matches, err := filepath.Glob(filepath.Join(sessionDir, strconv.Itoa(imageID)+".*"))
	if err != nil {
		return ImageBlob{}, err
	}
	if len(matches) == 0 {
		return ImageBlob{}, llm.WrapError("asset_store", llm.ErrorCodeAssetNotFound, fmt.Errorf("image %d for session %q not found", imageID, sessionID))
	}
	sort.Strings(matches)
	target := matches[0]
	if err := ensureWithinRoot(s.root, target); err != nil {
		return ImageBlob{}, err
	}

	data, err := os.ReadFile(target)
	if err != nil {
		if os.IsNotExist(err) {
			return ImageBlob{}, llm.WrapError("asset_store", llm.ErrorCodeAssetNotFound, err)
		}
		return ImageBlob{}, err
	}
	info, err := os.Stat(target)
	if err != nil {
		return ImageBlob{}, err
	}
	mediaType, ok := mediaTypeForPath(target)
	if !ok {
		return ImageBlob{}, llm.WrapError("asset_store", llm.ErrorCodeUnsupportedImage, fmt.Errorf("unsupported file extension: %s", filepath.Ext(target)))
	}
	rel, err := filepath.Rel(s.root, target)
	if err != nil {
		return ImageBlob{}, err
	}

	return ImageBlob{
		SessionID: sessionID,
		ImageID:   imageID,
		MediaType: mediaType,
		Data:      data,
		ByteSize:  info.Size(),
		CachePath: filepath.ToSlash(rel),
	}, nil
}

func parseAssetID(assetID llm.AssetID) (corepkg.SessionID, int, error) {
	raw := strings.TrimSpace(string(assetID))
	if raw == "" {
		return corepkg.SessionID(""), 0, fmt.Errorf("asset id is empty")
	}
	idx := strings.LastIndex(raw, ":")
	if idx < 0 || idx == len(raw)-1 {
		return corepkg.SessionID(""), 0, fmt.Errorf("invalid asset id %q", raw)
	}
	sessionID := strings.TrimSpace(raw[:idx])
	if sessionID == "" {
		return corepkg.SessionID(""), 0, fmt.Errorf("invalid asset id %q", raw)
	}
	id, err := strconv.Atoi(raw[idx+1:])
	if err != nil || id < 0 {
		return corepkg.SessionID(""), 0, fmt.Errorf("invalid asset id %q", raw)
	}
	return corepkg.SessionID(sessionID), id, nil
}

func (s *FileAssetStore) DeleteSessionImages(ctx context.Context, sessionID corepkg.SessionID) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	sanitized := sanitizeSessionID(string(sessionID))
	if sanitized == "" {
		return nil
	}
	target := filepath.Join(s.root, sanitized)
	if err := ensureWithinRoot(s.root, target); err != nil {
		return err
	}
	if _, err := os.Stat(target); os.IsNotExist(err) {
		return nil
	}
	return os.RemoveAll(target)
}

func (s *FileAssetStore) GC(ctx context.Context, keepSessionIDs []corepkg.SessionID, olderThan time.Time) error {
	keep := make(map[string]struct{}, len(keepSessionIDs))
	for _, id := range keepSessionIDs {
		sanitized := sanitizeSessionID(string(id))
		if sanitized != "" {
			keep[sanitized] = struct{}{}
		}
	}

	entries, err := os.ReadDir(s.root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if _, ok := keep[name]; ok {
			continue
		}
		target := filepath.Join(s.root, name)
		if err := ensureWithinRoot(s.root, target); err != nil {
			return err
		}
		if !olderThan.IsZero() {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if !info.ModTime().Before(olderThan) {
				continue
			}
		}
		if err := os.RemoveAll(target); err != nil {
			return err
		}
	}
	return nil
}

func AssetID(sessionID corepkg.SessionID, imageID int) llm.AssetID {
	return llm.AssetID(string(sessionID) + ":" + strconv.Itoa(imageID))
}

func extensionForMediaType(mediaType string) (string, bool) {
	switch strings.TrimSpace(strings.ToLower(mediaType)) {
	case "image/png":
		return "png", true
	case "image/jpeg", "image/jpg":
		return "jpg", true
	case "image/webp":
		return "webp", true
	case "image/gif":
		return "gif", true
	default:
		return "", false
	}
}

func mediaTypeForPath(path string) (string, bool) {
	switch strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".") {
	case "png":
		return "image/png", true
	case "jpg", "jpeg":
		return "image/jpeg", true
	case "webp":
		return "image/webp", true
	case "gif":
		return "image/gif", true
	default:
		return "", false
	}
}

func sanitizeSessionID(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(raw))
	for _, r := range raw {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	value := strings.Trim(b.String(), "_")
	if value == "" {
		return ""
	}
	return value
}

func ensureWithinRoot(root, target string) error {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	targetAbs, err := filepath.Abs(filepath.Clean(target))
	if err != nil {
		return err
	}
	rootWithSep := rootAbs + string(os.PathSeparator)
	if !strings.HasPrefix(strings.ToLower(targetAbs), strings.ToLower(rootWithSep)) && !strings.EqualFold(rootAbs, targetAbs) {
		return llm.WrapError("asset_store", llm.ErrorCodeUnknown, fmt.Errorf("path traversal rejected: %s", target))
	}
	return nil
}

func writeAtomicFile(path string, content []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	cleanup = false

	if dirHandle, err := os.Open(dir); err == nil {
		_ = dirHandle.Sync()
		_ = dirHandle.Close()
	}

	return nil
}
