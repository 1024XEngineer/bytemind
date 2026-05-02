package runtime

import (
	"context"
	"errors"
	"io"

	"github.com/1024XEngineer/bytemind/internal/agent"
	"github.com/1024XEngineer/bytemind/internal/assets"
	"github.com/1024XEngineer/bytemind/internal/history"
	"github.com/1024XEngineer/bytemind/internal/session"
)

var (
	errRunnerUnavailable = errors.New("runner is unavailable")
	errStoreUnavailable  = errors.New("session store is unavailable")
)

type Dependencies struct {
	Runner     *agent.Runner
	Store      *session.Store
	ImageStore assets.ImageStore
	Workspace  string
}

type UIAPI interface {
	LoadSkillCatalog(sess *session.Session) (SkillCatalog, error)
	ActivateSkill(sess *session.Session, name string, args map[string]string) (SkillActivation, error)
	ClearActiveSkill(sess *session.Session) (SkillClearResult, error)
	DeleteSkill(sess *session.Session, name string) (SkillDeleteResult, error)
	NewSession(workspace string) (*session.Session, error)
	ResumeSession(workspace, id string) (*session.Session, error)
	SaveSession(sess *session.Session) error
	ListSessions(limit int) ([]session.Summary, error)
	LoadRecentPrompts(limit int) ([]history.PromptEntry, error)
	CompactSession(sess *session.Session) (CompactSessionResult, error)
	ApplyStartupField(req StartupFieldRequest) (StartupFieldResult, error)
	VerifyStartupAPIKey(req StartupVerifyRequest) (StartupVerifyResult, error)
	EnsureSessionImageAssets(sess *session.Session)
	NextSessionImageID(sess *session.Session) int
	PutSessionImage(sess *session.Session, imageID int, mediaType, fileName string, data []byte) (StoredImage, error)
	PutSessionImageFromPath(sess *session.Session, imageID int, path string) (StoredImage, error)
	FindSessionAssetByImageID(sess *session.Session, imageID int) (StoredImageRef, bool)
	LoadSessionImageAsset(sess *session.Session, assetID string) (assets.ImageBlob, error)
	HydrateHistoricalAssets(sess *session.Session, current map[string]ImagePayload) map[string]ImagePayload
	LoadPastedContents(sess *session.Session) (PastedState, error)
	SavePastedContents(sess *session.Session, state PastedState) error
	RunPromptWithInput(ctx context.Context, sess *session.Session, prompt agent.RunPromptInput, mode string, sink io.Writer) error
}

type Service struct {
	runner     *agent.Runner
	store      *session.Store
	imageStore assets.ImageStore
	workspace  string
}

func NewService(deps Dependencies) *Service {
	return &Service{
		runner:     deps.Runner,
		store:      deps.Store,
		imageStore: deps.ImageStore,
		workspace:  deps.Workspace,
	}
}

func (s *Service) requireRunner() error {
	if s == nil || s.runner == nil {
		return errRunnerUnavailable
	}
	return nil
}

func (s *Service) requireStore() error {
	if s == nil || s.store == nil {
		return errStoreUnavailable
	}
	return nil
}

func cloneStringMap(source map[string]string) map[string]string {
	if len(source) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func (s *Service) RunPromptWithInput(ctx context.Context, sess *session.Session, prompt agent.RunPromptInput, mode string, sink io.Writer) error {
	if err := s.requireRunner(); err != nil {
		return err
	}
	if sink == nil {
		sink = io.Discard
	}
	_, err := s.runner.RunPromptWithInput(ctx, sess, prompt, mode, sink)
	return err
}
