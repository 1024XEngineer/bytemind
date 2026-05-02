package services

import (
	"errors"
	"strings"
	"testing"

	"github.com/1024XEngineer/bytemind/internal/assets"
	"github.com/1024XEngineer/bytemind/internal/llm"
	"github.com/1024XEngineer/bytemind/internal/session"
	tuiapi "github.com/1024XEngineer/bytemind/tui/api"
	tuiruntime "github.com/1024XEngineer/bytemind/tui/runtime"
)

type fakePromptBuilderAPI struct {
	*tuiruntime.Service
	refs  map[int]tuiruntime.StoredImageRef
	blobs map[string]assets.ImageBlob
}

func newFakePromptBuilderAPI() *fakePromptBuilderAPI {
	return &fakePromptBuilderAPI{
		Service: tuiruntime.NewService(tuiruntime.Dependencies{}),
		refs:    make(map[int]tuiruntime.StoredImageRef),
		blobs:   make(map[string]assets.ImageBlob),
	}
}

func (f *fakePromptBuilderAPI) FindSessionAssetByImageID(_ *session.Session, imageID int) (tuiruntime.StoredImageRef, bool) {
	ref, ok := f.refs[imageID]
	return ref, ok
}

func (f *fakePromptBuilderAPI) LoadSessionImageAsset(_ *session.Session, assetID string) (assets.ImageBlob, error) {
	blob, ok := f.blobs[assetID]
	if !ok {
		return assets.ImageBlob{}, errors.New("asset not found")
	}
	return blob, nil
}

func countImageParts(msg llm.Message) int {
	total := 0
	for _, part := range msg.Parts {
		if part.Type == llm.PartImageRef && part.Image != nil {
			total++
		}
	}
	return total
}

func TestPromptBuilderBuildParsesAtomicAndLegacyImagePlaceholders(t *testing.T) {
	api := newFakePromptBuilderAPI()
	assetID := llm.AssetID("sess:1")
	api.refs[1] = tuiruntime.StoredImageRef{AssetID: assetID}
	api.blobs[string(assetID)] = assets.ImageBlob{
		MediaType: "image/png",
		Data:      []byte("png-bytes"),
	}

	builder := NewPromptBuilder(api, session.New(t.TempDir()))
	cases := []string{"[Image#1]", "[Image #1]"}
	for _, placeholder := range cases {
		result := builder.Build(tuiapi.PromptBuildRequest{
			RawInput: "inspect " + placeholder + " only",
		}, tuiruntime.PastedState{})

		if !result.Success {
			t.Fatalf("expected success for %q, got error: %s", placeholder, result.Error)
		}
		if got := countImageParts(result.Data.Prompt.UserMessage); got != 1 {
			t.Fatalf("expected one image part for %q, got %d (message=%#v)", placeholder, got, result.Data.Prompt.UserMessage.Parts)
		}
		if len(result.Data.Prompt.Assets) != 1 {
			t.Fatalf("expected one asset payload for %q, got %d", placeholder, len(result.Data.Prompt.Assets))
		}
		if _, ok := result.Data.Prompt.Assets[assetID]; !ok {
			t.Fatalf("expected asset %q for %q", assetID, placeholder)
		}
		if strings.Contains(result.Data.Prompt.UserMessage.Text(), "unavailable") {
			t.Fatalf("did not expect unavailable fallback for %q, got %q", placeholder, result.Data.Prompt.UserMessage.Text())
		}
	}
}

func TestPromptBuilderBuildFallbackForAtomicAndLegacyUnknownPlaceholders(t *testing.T) {
	api := tuiruntime.NewService(tuiruntime.Dependencies{})
	builder := NewPromptBuilder(api, session.New(t.TempDir()))

	cases := []string{"[Image#1]", "[Image #1]"}
	for _, placeholder := range cases {
		result := builder.Build(tuiapi.PromptBuildRequest{
			RawInput: "inspect " + placeholder + " only",
		}, tuiruntime.PastedState{})

		if !result.Success {
			t.Fatalf("expected success for %q, got error: %s", placeholder, result.Error)
		}
		if got := countImageParts(result.Data.Prompt.UserMessage); got != 0 {
			t.Fatalf("expected no image part for missing asset %q, got %d", placeholder, got)
		}
		if len(result.Data.Prompt.Assets) != 0 {
			t.Fatalf("expected no assets for missing placeholder %q, got %d", placeholder, len(result.Data.Prompt.Assets))
		}
		if !strings.Contains(result.Data.Prompt.UserMessage.Text(), "Image #1 unavailable") {
			t.Fatalf("expected unavailable fallback for %q, got %q", placeholder, result.Data.Prompt.UserMessage.Text())
		}
	}
}
