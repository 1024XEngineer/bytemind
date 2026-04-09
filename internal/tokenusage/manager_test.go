package tokenusage

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTokenUsageManager_RecordTokenUsage(t *testing.T) {
	tm := mustNewManager(t, Config{
		StorageType:    "memory",
		BackupInterval: 200 * time.Millisecond,
		RetentionDays:  7,
		EnableRealtime: true,
	})
	defer tm.Close()

	err := tm.RecordTokenUsage(context.Background(), &TokenRecordRequest{
		SessionID:    "s-1",
		ModelName:    "gpt-5.4",
		InputTokens:  120,
		OutputTokens: 30,
		RequestID:    "r-1",
		Latency:      300 * time.Millisecond,
		Success:      true,
		Metadata: map[string]string{
			"workspace": "repo-a",
		},
	})
	if err != nil {
		t.Fatalf("record usage: %v", err)
	}

	err = tm.RecordTokenUsage(context.Background(), &TokenRecordRequest{
		SessionID:    "s-1",
		ModelName:    "gpt-5.4",
		InputTokens:  80,
		OutputTokens: 20,
		RequestID:    "r-2",
		Latency:      100 * time.Millisecond,
		Success:      true,
	})
	if err != nil {
		t.Fatalf("record usage(2): %v", err)
	}

	stats, err := tm.GetSessionStats("s-1")
	if err != nil {
		t.Fatalf("get session stats: %v", err)
	}
	if stats.TotalTokens != 250 || stats.InputTokens != 200 || stats.OutputTokens != 50 {
		t.Fatalf("unexpected session tokens: %+v", stats)
	}
	if stats.RequestCount != 2 {
		t.Fatalf("expected request count 2, got %d", stats.RequestCount)
	}
}

func TestTokenUsageManager_GetRealtimeStats(t *testing.T) {
	tm := mustNewManager(t, Config{
		StorageType:    "memory",
		BackupInterval: 200 * time.Millisecond,
		RetentionDays:  7,
		EnableRealtime: true,
	})
	defer tm.Close()

	_ = tm.RecordTokenUsage(context.Background(), &TokenRecordRequest{
		SessionID:    "s-a",
		ModelName:    "gpt-5.4",
		InputTokens:  10,
		OutputTokens: 5,
		Success:      true,
		Latency:      200 * time.Millisecond,
	})
	_ = tm.RecordTokenUsage(context.Background(), &TokenRecordRequest{
		SessionID:    "s-b",
		ModelName:    "gpt-5.4-mini",
		InputTokens:  20,
		OutputTokens: 6,
		Success:      false,
		Latency:      300 * time.Millisecond,
	})

	rs, err := tm.GetRealtimeStats()
	if err != nil {
		t.Fatalf("get realtime stats: %v", err)
	}
	if rs.Metrics.ActiveSessions != 2 {
		t.Fatalf("expected active sessions=2, got %d", rs.Metrics.ActiveSessions)
	}
	if rs.TotalTokens != 41 {
		t.Fatalf("expected total tokens 41, got %d", rs.TotalTokens)
	}
	if rs.Metrics.ErrorRate <= 0 {
		t.Fatalf("expected non-zero error rate, got %f", rs.Metrics.ErrorRate)
	}
}

func TestTokenUsageManager_GenerateReport(t *testing.T) {
	tm := mustNewManager(t, Config{
		StorageType:    "memory",
		BackupInterval: 200 * time.Millisecond,
		RetentionDays:  7,
		EnableRealtime: true,
	})
	defer tm.Close()

	now := time.Now().UTC()
	_ = tm.RecordTokenUsage(context.Background(), &TokenRecordRequest{
		SessionID:    "session-report",
		ModelName:    "gpt-5.4",
		InputTokens:  50,
		OutputTokens: 25,
		Success:      true,
		Latency:      100 * time.Millisecond,
		Metadata:     map[string]string{"workspace": "alpha"},
	})
	_ = tm.RecordTokenUsage(context.Background(), &TokenRecordRequest{
		SessionID:    "session-report",
		ModelName:    "gpt-5.4",
		InputTokens:  30,
		OutputTokens: 10,
		Success:      true,
		Latency:      80 * time.Millisecond,
		Metadata:     map[string]string{"workspace": "alpha"},
	})

	report, err := tm.GenerateReport(&ReportRequest{
		Type:      "custom",
		StartDate: now.Add(-time.Hour),
		EndDate:   now.Add(time.Hour),
		Filters: &UsageFilter{
			SessionIDs: []string{"session-report"},
		},
	})
	if err != nil {
		t.Fatalf("generate report: %v", err)
	}
	if report.TotalTokens != 115 {
		t.Fatalf("expected total tokens 115, got %d", report.TotalTokens)
	}
	if report.RequestCount != 2 {
		t.Fatalf("expected request count 2, got %d", report.RequestCount)
	}

	raw, err := tm.ExportData(&ExportRequest{
		Format: "csv",
		Report: &ReportRequest{
			Type:      "custom",
			StartDate: now.Add(-time.Hour),
			EndDate:   now.Add(time.Hour),
		},
	})
	if err != nil {
		t.Fatalf("export csv: %v", err)
	}
	if !strings.Contains(string(raw), "total_tokens") {
		t.Fatalf("expected csv output to include total_tokens header, got %q", string(raw))
	}
}

func TestTokenUsageManager_Integration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token_usage.json")
	cfg := Config{
		StorageType:    "file",
		StoragePath:    path,
		BackupInterval: 150 * time.Millisecond,
		RetentionDays:  30,
		EnableRealtime: true,
	}

	tm := mustNewManager(t, cfg)
	_ = tm.RecordTokenUsage(context.Background(), &TokenRecordRequest{
		SessionID:    "integration-session",
		ModelName:    "gpt-5.4",
		InputTokens:  40,
		OutputTokens: 12,
		RequestID:    "int-1",
		Latency:      120 * time.Millisecond,
		Success:      true,
	})
	if err := tm.Close(); err != nil {
		t.Fatalf("close manager: %v", err)
	}

	tm2 := mustNewManager(t, cfg)
	defer tm2.Close()
	stats, err := tm2.GetSessionStats("integration-session")
	if err != nil {
		t.Fatalf("load persisted session stats: %v", err)
	}
	if stats.TotalTokens != 52 {
		t.Fatalf("expected persisted total tokens 52, got %d", stats.TotalTokens)
	}
}

func BenchmarkTokenUsageManager_RecordTokenUsage(b *testing.B) {
	tm, err := NewTokenUsageManager(&Config{
		StorageType:    "memory",
		BackupInterval: time.Second,
		RetentionDays:  14,
		EnableRealtime: true,
		MaxSessions:    50000,
	})
	if err != nil {
		b.Fatalf("new manager: %v", err)
	}
	defer tm.Close()

	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = tm.RecordTokenUsage(ctx, &TokenRecordRequest{
			SessionID:    "bench-session",
			ModelName:    "gpt-5.4-mini",
			InputTokens:  15,
			OutputTokens: 7,
			RequestID:    "bench",
			Latency:      20 * time.Millisecond,
			Success:      true,
		})
	}
}

func mustNewManager(t *testing.T, cfg Config) *TokenUsageManager {
	t.Helper()
	mgr, err := NewTokenUsageManager(&cfg)
	if err != nil {
		t.Fatalf("new token usage manager: %v", err)
	}
	return mgr
}
