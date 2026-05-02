package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/1024XEngineer/bytemind/internal/assets"
	corepkg "github.com/1024XEngineer/bytemind/internal/core"
	"github.com/1024XEngineer/bytemind/internal/llm"
	"github.com/1024XEngineer/bytemind/internal/session"
)

type StoredImage struct {
	AssetID   llm.AssetID
	ImageID   int
	MediaType string
	FileName  string
	CachePath string
	ByteSize  int64
	Width     int
	Height    int
}

type StoredImageRef struct {
	AssetID llm.AssetID
	Meta    session.ImageAssetMeta
}

type ImagePayload struct {
	MediaType string
	Data      []byte
}

func (s *Service) EnsureSessionImageAssets(sess *session.Session) {
	EnsureSessionImageAssets(sess)
}

func EnsureSessionImageAssets(sess *session.Session) {
	if sess == nil {
		return
	}
	if sess.Conversation.Assets.Images == nil {
		sess.Conversation.Assets.Images = make(map[llm.AssetID]session.ImageAssetMeta, 8)
	}
}

func (s *Service) NextSessionImageID(sess *session.Session) int {
	return NextSessionImageID(sess)
}

func NextSessionImageID(sess *session.Session) int {
	if sess == nil {
		return 1
	}
	maxID := 0
	for _, meta := range sess.Conversation.Assets.Images {
		if meta.ImageID > maxID {
			maxID = meta.ImageID
		}
	}
	if maxID < 0 {
		return 1
	}
	return maxID + 1
}

func (s *Service) PutSessionImage(sess *session.Session, imageID int, mediaType, fileName string, data []byte) (StoredImage, error) {
	if sess == nil {
		return StoredImage{}, fmt.Errorf("image ingest failed: session unavailable")
	}
	if s == nil || s.imageStore == nil {
		return StoredImage{}, fmt.Errorf("image ingest failed: image store unavailable")
	}
	if len(data) == 0 {
		return StoredImage{}, fmt.Errorf("image ingest failed: empty image payload")
	}

	s.EnsureSessionImageAssets(sess)
	sessionID := corepkg.SessionID(sess.ID)
	meta, err := s.imageStore.PutImage(context.Background(), assets.PutImageInput{
		SessionID: sessionID,
		ImageID:   imageID,
		MediaType: mediaType,
		FileName:  fileName,
		Data:      data,
	})
	if err != nil {
		return StoredImage{}, err
	}

	assetID := meta.AssetID
	if strings.TrimSpace(string(assetID)) == "" {
		assetID = assets.AssetID(sessionID, meta.ImageID)
	}
	sess.Conversation.Assets.Images[assetID] = session.ImageAssetMeta{
		ImageID:   meta.ImageID,
		MediaType: meta.MediaType,
		FileName:  meta.FileName,
		CachePath: meta.CachePath,
		ByteSize:  meta.ByteSize,
		Width:     meta.Width,
		Height:    meta.Height,
	}
	if err := s.SaveSession(sess); err != nil {
		return StoredImage{}, err
	}

	return StoredImage{
		AssetID:   assetID,
		ImageID:   meta.ImageID,
		MediaType: meta.MediaType,
		FileName:  meta.FileName,
		CachePath: meta.CachePath,
		ByteSize:  meta.ByteSize,
		Width:     meta.Width,
		Height:    meta.Height,
	}, nil
}

func (s *Service) PutSessionImageFromPath(sess *session.Session, imageID int, path string) (StoredImage, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return StoredImage{}, fmt.Errorf("failed to read image %s: %v", path, err)
	}
	mediaType, ok := mediaTypeFromPath(path)
	if !ok {
		return StoredImage{}, fmt.Errorf("unsupported image format: %s", filepath.Ext(path))
	}
	return s.PutSessionImage(sess, imageID, mediaType, filepath.Base(path), data)
}

func (s *Service) FindSessionAssetByImageID(sess *session.Session, imageID int) (StoredImageRef, bool) {
	if sess == nil {
		return StoredImageRef{}, false
	}
	for assetID, meta := range sess.Conversation.Assets.Images {
		if meta.ImageID == imageID {
			return StoredImageRef{AssetID: assetID, Meta: meta}, true
		}
	}
	return StoredImageRef{}, false
}

func (s *Service) LoadSessionImageAsset(sess *session.Session, assetID string) (assets.ImageBlob, error) {
	if sess == nil {
		return assets.ImageBlob{}, fmt.Errorf("session is unavailable")
	}
	if s == nil || s.imageStore == nil {
		return assets.ImageBlob{}, fmt.Errorf("image store unavailable")
	}
	return s.imageStore.GetImageByAssetID(context.Background(), corepkg.SessionID(sess.ID), llm.AssetID(assetID))
}

func (s *Service) HydrateHistoricalAssets(sess *session.Session, current map[string]ImagePayload) map[string]ImagePayload {
	if sess == nil || s == nil || s.imageStore == nil {
		return current
	}
	imageAssetIDs := collectImageAssetIDsFromMessages(sess.Messages)
	if len(imageAssetIDs) == 0 {
		return current
	}
	if current == nil {
		current = make(map[string]ImagePayload, len(imageAssetIDs))
	}
	for _, assetID := range imageAssetIDs {
		key := strings.TrimSpace(string(assetID))
		if key == "" {
			continue
		}
		if _, ok := current[key]; ok {
			continue
		}
		blob, err := s.imageStore.GetImageByAssetID(context.Background(), corepkg.SessionID(sess.ID), assetID)
		if err != nil || len(blob.Data) == 0 {
			continue
		}
		current[key] = ImagePayload{
			MediaType: blob.MediaType,
			Data:      blob.Data,
		}
	}
	if len(current) == 0 {
		return nil
	}
	return current
}

func collectImageAssetIDsFromMessages(messages []llm.Message) []llm.AssetID {
	if len(messages) == 0 {
		return nil
	}
	seen := make(map[llm.AssetID]struct{}, 8)
	assetIDs := make([]llm.AssetID, 0, 8)
	for i := range messages {
		msg := messages[i]
		msg.Normalize()
		for _, part := range msg.Parts {
			if part.Type != llm.PartImageRef || part.Image == nil {
				continue
			}
			assetID := part.Image.AssetID
			if strings.TrimSpace(string(assetID)) == "" {
				continue
			}
			if _, ok := seen[assetID]; ok {
				continue
			}
			seen[assetID] = struct{}{}
			assetIDs = append(assetIDs, assetID)
		}
	}
	if len(assetIDs) == 0 {
		return nil
	}
	return assetIDs
}

func mediaTypeFromPath(path string) (string, bool) {
	switch strings.ToLower(strings.TrimPrefix(filepath.Ext(strings.TrimSpace(path)), ".")) {
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
