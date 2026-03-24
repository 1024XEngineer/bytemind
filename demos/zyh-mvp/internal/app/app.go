package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"forgecli/internal/config"
	"forgecli/internal/editor"
	"forgecli/internal/executor"
	"forgecli/internal/model"
	"forgecli/internal/planner"
	"forgecli/internal/policy"
	"forgecli/internal/report"
	"forgecli/internal/session"
	"forgecli/internal/ui"
	"forgecli/internal/workspace"
)

type Params struct {
	RepoPath      string
	Task          string
	VerifyCommand string
}

type App struct {
	cfg      config.Config
	terminal ui.Terminal
	logger   *slog.Logger

	planner planner.Planner
	model   model.Provider
	editor  editor.Service
	runner  executor.Runner
	policy  policy.Policy
}

func New(cfg config.Config, terminal ui.Terminal, logger *slog.Logger) *App {
	if logger == nil {
		logger = slog.Default()
	}

	return &App{
		cfg:      cfg,
		terminal: terminal,
		logger:   logger,
		planner:  planner.MockPlanner{},
		model:    buildProvider(cfg, logger),
		editor:   editor.Service{},
		runner: executor.Runner{
			Timeout: time.Duration(cfg.CommandTimeoutSeconds) * time.Second,
		},
		policy: policy.New(cfg),
	}
}

func (a *App) Run(ctx context.Context, params Params) error {
	if strings.TrimSpace(params.RepoPath) == "" {
		return errors.New("repo path is required")
	}
	if strings.TrimSpace(params.Task) == "" {
		return errors.New("task is required")
	}

	ws, err := workspace.Open(params.RepoPath, a.cfg.SearchIgnore, a.cfg.SensitivePatterns)
	if err != nil {
		return err
	}

	current := session.New(params.Task, ws.Root, params.VerifyCommand)
	a.terminal.Println("ForgeCLI MVP Demo")
	a.terminal.Println("=================")
	a.terminal.Printf("Workspace: %s\n", ws.Root)
	a.terminal.Printf("Task: %s\n\n", params.Task)

	plan, err := a.planner.BuildPlan(ctx, params.Task, ws)
	if err != nil {
		return err
	}
	current.Plan = plan
	current.TargetFile = plan.TargetFile

	a.terminal.Println("计划:")
	a.terminal.Printf("- %s\n", plan.Summary)
	for _, step := range plan.Steps {
		a.terminal.Printf("  * %s\n", step)
	}
	if len(plan.SearchHits) > 0 {
		a.terminal.Println("\n上下文命中:")
		for _, hit := range plan.SearchHits {
			a.terminal.Printf("- %s:%d %s\n", hit.Path, hit.Line, hit.Preview)
		}
	}

	if err := a.policy.EnsureEditablePath(plan.TargetFile); err != nil {
		return err
	}

	readResult, err := ws.ReadFile(plan.TargetFile)
	if err != nil {
		return err
	}
	if readResult.Sensitive {
		return fmt.Errorf("refusing to edit sensitive file: %s", plan.TargetFile)
	}

	change, err := a.model.ProposeChange(params.Task, plan.TargetFile, readResult.Content)
	if err != nil {
		return err
	}

	if change.Noop || change.NewContent == string(readResult.Content) {
		if change.Summary == "" {
			change.Summary = "当前任务无需修改文件。"
		}
		current.AddNote(change.Summary)
		return a.finishWithOptionalVerify(ctx, current, ws.Root, params.VerifyCommand)
	}

	diffPreview := a.editor.Preview(readResult.Content, []byte(change.NewContent))
	a.terminal.Printf("\n变更预览 (%s):\n%s\n\n", plan.TargetFile, diffPreview)

	writeApproved, err := a.terminal.PromptYesNo("是否写入这个变更？")
	if err != nil {
		return err
	}
	current.WriteApproved = writeApproved
	if !writeApproved {
		current.AddNote("用户拒绝写入，任务在 diff 预览阶段结束。")
		a.terminal.Printf("\n%s\n", report.Render(current))
		return nil
	}

	targetAbs, err := ws.Resolve(plan.TargetFile)
	if err != nil {
		return err
	}
	if err := a.editor.Apply(targetAbs, readResult.Hash, []byte(change.NewContent)); err != nil {
		if errors.Is(err, editor.ErrConflict) {
			current.AddNote("文件在写入前已被外部修改，请重新读取后再尝试。")
		}
		a.terminal.Printf("\n%s\n", report.Render(current))
		return err
	}

	current.FileWritten = true
	current.ChangedFiles = append(current.ChangedFiles, plan.TargetFile)
	current.AddNote(change.Summary)

	return a.finishWithOptionalVerify(ctx, current, ws.Root, params.VerifyCommand)
}

func (a *App) finishWithOptionalVerify(ctx context.Context, current *session.Session, workspaceRoot, verifyCommand string) error {
	if strings.TrimSpace(verifyCommand) == "" {
		current.AddNote("未提供验证命令，跳过验证步骤。")
		a.terminal.Printf("\n%s\n", report.Render(current))
		return nil
	}

	if err := a.policy.ValidateCommand(verifyCommand); err != nil {
		current.AddNote("验证命令被策略拦截。")
		a.terminal.Printf("\n%s\n", report.Render(current))
		return err
	}

	verifyApproved, err := a.terminal.PromptYesNo(fmt.Sprintf("是否执行验证命令 %q？", verifyCommand))
	if err != nil {
		return err
	}
	current.VerifyApproved = verifyApproved
	if !verifyApproved {
		current.AddNote("用户拒绝执行验证命令。")
		a.terminal.Printf("\n%s\n", report.Render(current))
		return nil
	}

	a.logger.Info("running verify command", "command", verifyCommand, "workspace", workspaceRoot)
	verifyResult, runErr := a.runner.Run(ctx, workspaceRoot, verifyCommand)
	current.VerifyResult = &verifyResult
	if runErr != nil {
		current.AddNote(runErr.Error())
	} else {
		current.AddNote("验证命令执行完成。")
	}

	a.terminal.Printf("\n%s\n", report.Render(current))
	if runErr != nil {
		return runErr
	}
	if verifyResult.ExitCode != 0 {
		return fmt.Errorf("verify command exited with code %d", verifyResult.ExitCode)
	}

	return nil
}

func buildProvider(cfg config.Config, logger *slog.Logger) model.Provider {
	if !shouldUseLLMForRun(cfg.Model) {
		return model.StubProvider{}
	}

	client, err := model.NewChatClient(cfg.Model)
	if err != nil {
		if logger != nil {
			logger.Warn("failed to initialize run model provider, falling back to stub", "error", err)
		}
		return model.StubProvider{}
	}

	provider, err := model.NewLLMProvider(cfg.Model, client)
	if err != nil {
		if logger != nil {
			logger.Warn("failed to build llm provider, falling back to stub", "error", err)
		}
		return model.StubProvider{}
	}

	return provider
}

func shouldUseLLMForRun(cfg config.ModelConfig) bool {
	if strings.TrimSpace(cfg.Model) == "" {
		return false
	}
	if strings.TrimSpace(cfg.APIKey) != "" {
		return true
	}
	if envName := strings.TrimSpace(cfg.APIKeyEnv); envName != "" && strings.TrimSpace(os.Getenv(envName)) != "" {
		return true
	}
	return strings.TrimSpace(cfg.BaseURL) != "" && strings.TrimSpace(cfg.BaseURL) != "https://api.openai.com/v1"
}
