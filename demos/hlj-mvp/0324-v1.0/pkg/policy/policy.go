package policy

import (
	"encoding/json"
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

func (p *Policy) IsSensitiveFile(filename string) bool {
	lower := strings.ToLower(filename)
	for _, pattern := range sensitivePatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}

func (p *Policy) Log(action, target string, approved, success bool, details string) {
	audit := AuditLog{
		SessionID: "",
		Timestamp: time.Now(),
		Action:    action,
		Target:    target,
		Approved:  approved,
		Success:   success,
		Details:   details,
	}

	data, _ := json.Marshal(audit)
	auditPath := filepath.Join(p.workspace, ".forgecli", "audit.jsonl")
	os.MkdirAll(filepath.Dir(auditPath), 0755)
	f, _ := os.OpenFile(auditPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	defer f.Close()
	f.WriteString(string(data) + "\n")
}

func (p *Policy) CanRead(path string) bool {
	if p.IsSensitiveFile(path) {
		return false
	}
	return true
}

func (p *Policy) CanWrite(path string) bool {
	if p.IsSensitiveFile(path) {
		return false
	}
	return true
}

func (p *Policy) CanExec(cmd string) bool {
	dangerous := []string{
		"rm -rf",
		"del /s /q",
		"format",
	}

	lower := strings.ToLower(cmd)
	for _, d := range dangerous {
		if strings.Contains(lower, d) {
			return false
		}
	}
	return true
}
