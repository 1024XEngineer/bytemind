package policy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type AuditLog struct {
	SessionID string    `json:"session_id"`
	Timestamp time.Time `json:"timestamp"`
	Action    string    `json:"action"`
	Target    string    `json:"target"`
	Approved  bool      `json:"approved"`
	Success   bool      `json:"success"`
	Details   string    `json:"details"`
}

type Policy struct {
	workspace string
}

func New(workspace string) *Policy {
	return &Policy{workspace: workspace}
}

var sensitivePatterns = []string{
	".env",
	".env.local",
	".env.production",
	"id_rsa",
	"id_ed25519",
	".pem",
	".key",
	"credentials.json",
	"secrets.yaml",
	".gitcredentials",
}

var blockedCommandPatterns = []string{
	"rm -rf",
	"del /s /q",
	"remove-item -recurse -force",
	"format ",
	"format.com",
	"rmdir /s",
	"shutdown",
	"reboot",
	"poweroff",
	"mkfs",
	"diskpart",
	"git reset --hard",
	"git checkout --",
	"git config --global",
	"git config --system",
	"safe.directory",
}

func (p *Policy) IsSensitiveFile(filename string) bool {
	lower := strings.ToLower(filename)
	for _, pattern := range sensitivePatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}

	return false
}

func (p *Policy) CheckRead(path string) error {
	if p.IsSensitiveFile(path) {
		return fmt.Errorf("sensitive file is protected: %s", path)
	}

	return nil
}

func (p *Policy) CheckWrite(path string) error {
	if p.IsSensitiveFile(path) {
		return fmt.Errorf("sensitive file is protected: %s", path)
	}

	return nil
}

func (p *Policy) CheckDelete(path string) error {
	return p.CheckWrite(path)
}

func (p *Policy) CheckExec(cmd string) error {
	lower := strings.ToLower(cmd)
	for _, pattern := range blockedCommandPatterns {
		if strings.Contains(lower, pattern) {
			return fmt.Errorf("blocked command: %s", cmd)
		}
	}

	return nil
}

func (p *Policy) CanRead(path string) bool {
	return p.CheckRead(path) == nil
}

func (p *Policy) CanWrite(path string) bool {
	return p.CheckWrite(path) == nil
}

func (p *Policy) CanExec(cmd string) bool {
	return p.CheckExec(cmd) == nil
}

func (p *Policy) Log(sessionID, action, target string, approved, success bool, details string) error {
	audit := AuditLog{
		SessionID: sessionID,
		Timestamp: time.Now(),
		Action:    action,
		Target:    target,
		Approved:  approved,
		Success:   success,
		Details:   details,
	}

	data, err := json.Marshal(audit)
	if err != nil {
		return err
	}

	auditPath := filepath.Join(p.workspace, ".forgecli", "audit.jsonl")
	if err := os.MkdirAll(filepath.Dir(auditPath), 0755); err != nil {
		return err
	}

	file, err := os.OpenFile(auditPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.WriteString(string(data) + "\n")
	return err
}
