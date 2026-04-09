package tokenusage

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"
)

// TokenUsageManager token使用量管理器。
type TokenUsageManager struct {
	mu             sync.RWMutex
	config         Config
	storage        Storage
	metrics        *Metrics
	realtimeStats  map[string]*SessionStats
	historicalData *HistoricalData
	notifier       Notifier
	logger         *slog.Logger

	dirtySessions   map[string]struct{}
	dirtyHistorical bool

	totalRequests int64
	totalErrors   int64
	totalLatency  time.Duration
	tpsSamples    []tpsSample

	stopCh   chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup

	monitorMu sync.Mutex
	monitorOn bool
}

type tpsSample struct {
	Time   time.Time
	Tokens int64
}

// Config 配置结构。
type Config struct {
	StorageType     string        `json:"storage_type"` // memory/file/database
	StoragePath     string        `json:"storage_path"`
	BackupInterval  time.Duration `json:"backup_interval"`
	MaxSessions     int           `json:"max_sessions"`
	AlertThreshold  int64         `json:"alert_threshold"`
	EnableRealtime  bool          `json:"enable_realtime"`
	RetentionDays   int           `json:"retention_days"`
	MonitorInterval time.Duration `json:"monitor_interval,omitempty"`
	BatchSize       int           `json:"batch_size,omitempty"`
	DatabaseDriver  string        `json:"database_driver,omitempty"`
}

// SessionStats 会话统计。
type SessionStats struct {
	SessionID     string            `json:"session_id"`
	StartTime     time.Time         `json:"start_time"`
	LastUpdate    time.Time         `json:"last_update"`
	InputTokens   int64             `json:"input_tokens"`
	OutputTokens  int64             `json:"output_tokens"`
	TotalTokens   int64             `json:"total_tokens"`
	RequestCount  int               `json:"request_count"`
	AverageTokens float64           `json:"average_tokens"`
	Metadata      map[string]string `json:"metadata"`
}

// HistoricalData 历史数据。
type HistoricalData struct {
	DailyStats   map[string]*DailyStats   `json:"daily_stats"`   // 按日期
	WeeklyStats  map[string]*WeeklyStats  `json:"weekly_stats"`  // 按周
	MonthlyStats map[string]*MonthlyStats `json:"monthly_stats"` // 按月
	ModelStats   map[string]*ModelStats   `json:"model_stats"`   // 按模型
	Records      []*UsageRecord           `json:"records,omitempty"`
}

// Metrics 实时指标。
type Metrics struct {
	CurrentTPS     float64       `json:"current_tps"` // 每秒token数
	PeakTPS        float64       `json:"peak_tps"`
	ActiveSessions int           `json:"active_sessions"`
	ErrorRate      float64       `json:"error_rate"`
	Latency        time.Duration `json:"avg_latency"`
}

type DailyStats struct {
	Date         string           `json:"date"`
	InputTokens  int64            `json:"input_tokens"`
	OutputTokens int64            `json:"output_tokens"`
	TotalTokens  int64            `json:"total_tokens"`
	RequestCount int64            `json:"request_count"`
	SuccessCount int64            `json:"success_count"`
	ErrorCount   int64            `json:"error_count"`
	AvgLatencyMs float64          `json:"avg_latency_ms"`
	ModelTokens  map[string]int64 `json:"model_tokens,omitempty"`
}

type WeeklyStats struct {
	Week         string           `json:"week"`
	InputTokens  int64            `json:"input_tokens"`
	OutputTokens int64            `json:"output_tokens"`
	TotalTokens  int64            `json:"total_tokens"`
	RequestCount int64            `json:"request_count"`
	SuccessCount int64            `json:"success_count"`
	ErrorCount   int64            `json:"error_count"`
	AvgLatencyMs float64          `json:"avg_latency_ms"`
	ModelTokens  map[string]int64 `json:"model_tokens,omitempty"`
}

type MonthlyStats struct {
	Month        string           `json:"month"`
	InputTokens  int64            `json:"input_tokens"`
	OutputTokens int64            `json:"output_tokens"`
	TotalTokens  int64            `json:"total_tokens"`
	RequestCount int64            `json:"request_count"`
	SuccessCount int64            `json:"success_count"`
	ErrorCount   int64            `json:"error_count"`
	AvgLatencyMs float64          `json:"avg_latency_ms"`
	ModelTokens  map[string]int64 `json:"model_tokens,omitempty"`
}

type ModelStats struct {
	ModelName    string    `json:"model_name"`
	InputTokens  int64     `json:"input_tokens"`
	OutputTokens int64     `json:"output_tokens"`
	TotalTokens  int64     `json:"total_tokens"`
	RequestCount int64     `json:"request_count"`
	SuccessCount int64     `json:"success_count"`
	ErrorCount   int64     `json:"error_count"`
	AvgLatencyMs float64   `json:"avg_latency_ms"`
	LastUpdated  time.Time `json:"last_updated"`
}

// Storage 存储接口。
type Storage interface {
	SaveSession(sessionID string, stats *SessionStats) error
	LoadSession(sessionID string) (*SessionStats, error)
	SaveHistorical(data *HistoricalData) error
	LoadHistorical() (*HistoricalData, error)
	ListSessions(start, end time.Time) ([]*SessionStats, error)
	DeleteSession(sessionID string) error
	Cleanup() error
}

type Notifier interface {
	Notify(ctx context.Context, alert *Alert) error
}

type RealtimeStats struct {
	Metrics     Metrics         `json:"metrics"`
	Sessions    []*SessionStats `json:"sessions"`
	GeneratedAt time.Time       `json:"generated_at"`
	TotalTokens int64           `json:"total_tokens"`
}

// TokenRecordRequest token记录请求。
type TokenRecordRequest struct {
	SessionID    string            `json:"session_id"`
	ModelName    string            `json:"model_name"`
	InputTokens  int64             `json:"input_tokens"`
	OutputTokens int64             `json:"output_tokens"`
	RequestID    string            `json:"request_id"`
	Latency      time.Duration     `json:"latency"`
	Success      bool              `json:"success"`
	Metadata     map[string]string `json:"metadata"`
}

type UsageRecord struct {
	Time         time.Time         `json:"time"`
	SessionID    string            `json:"session_id"`
	ModelName    string            `json:"model_name"`
	InputTokens  int64             `json:"input_tokens"`
	OutputTokens int64             `json:"output_tokens"`
	TotalTokens  int64             `json:"total_tokens"`
	RequestID    string            `json:"request_id"`
	Latency      time.Duration     `json:"latency"`
	Success      bool              `json:"success"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

type UsageFilter struct {
	SessionIDs []string          `json:"session_ids,omitempty"`
	ModelNames []string          `json:"model_names,omitempty"`
	Success    *bool             `json:"success,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

type UsageReport struct {
	Type              string                  `json:"type"`
	StartDate         time.Time               `json:"start_date"`
	EndDate           time.Time               `json:"end_date"`
	TotalInputTokens  int64                   `json:"total_input_tokens"`
	TotalOutputTokens int64                   `json:"total_output_tokens"`
	TotalTokens       int64                   `json:"total_tokens"`
	RequestCount      int64                   `json:"request_count"`
	SuccessCount      int64                   `json:"success_count"`
	ErrorCount        int64                   `json:"error_count"`
	AvgLatency        time.Duration           `json:"avg_latency"`
	SessionStats      map[string]SessionStats `json:"session_stats,omitempty"`
	ModelStats        map[string]ModelStats   `json:"model_stats,omitempty"`
	DailyStats        map[string]DailyStats   `json:"daily_stats,omitempty"`
	GeneratedAt       time.Time               `json:"generated_at"`
}

type ModelUsageReport struct {
	ModelName         string        `json:"model_name"`
	Period            string        `json:"period"`
	TotalInputTokens  int64         `json:"total_input_tokens"`
	TotalOutputTokens int64         `json:"total_output_tokens"`
	TotalTokens       int64         `json:"total_tokens"`
	RequestCount      int64         `json:"request_count"`
	SuccessRate       float64       `json:"success_rate"`
	AverageTokens     float64       `json:"average_tokens"`
	AvgLatency        time.Duration `json:"avg_latency"`
	StartDate         time.Time     `json:"start_date"`
	EndDate           time.Time     `json:"end_date"`
}

// ReportRequest 报表请求。
type ReportRequest struct {
	Type      string       `json:"type"` // daily/weekly/monthly/custom
	StartDate time.Time    `json:"start_date"`
	EndDate   time.Time    `json:"end_date"`
	Format    string       `json:"format"` // json/csv/excel
	Filters   *UsageFilter `json:"filters"`
}

type ExportRequest struct {
	Format string         `json:"format"` // json/csv/excel
	Report *ReportRequest `json:"report"`
}

// AlertResult 告警结果。
type AlertResult struct {
	Alerts []*Alert `json:"alerts"`
}

type Alert struct {
	Type    string    `json:"type"`  // usage_quota/error_rate/latency
	Level   string    `json:"level"` // warning/critical
	Message string    `json:"message"`
	Value   float64   `json:"value"`
	Time    time.Time `json:"time"`
}

// DefaultConfig returns a pragmatic production-safe default.
func DefaultConfig() Config {
	return Config{
		StorageType:     "memory",
		StoragePath:     "",
		BackupInterval:  time.Minute,
		MaxSessions:     10000,
		AlertThreshold:  1000000,
		EnableRealtime:  true,
		RetentionDays:   30,
		MonitorInterval: 30 * time.Second,
		BatchSize:       200,
		DatabaseDriver:  "sqlite3",
	}
}

func cloneMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneSessionStats(in *SessionStats) *SessionStats {
	if in == nil {
		return nil
	}
	out := *in
	out.Metadata = cloneMap(in.Metadata)
	return &out
}

func cloneHistoricalData(in *HistoricalData) *HistoricalData {
	if in == nil {
		return newHistoricalData()
	}
	data, err := json.Marshal(in)
	if err != nil {
		return newHistoricalData()
	}
	var out HistoricalData
	if err := json.Unmarshal(data, &out); err != nil {
		return newHistoricalData()
	}
	if out.DailyStats == nil {
		out.DailyStats = map[string]*DailyStats{}
	}
	if out.WeeklyStats == nil {
		out.WeeklyStats = map[string]*WeeklyStats{}
	}
	if out.MonthlyStats == nil {
		out.MonthlyStats = map[string]*MonthlyStats{}
	}
	if out.ModelStats == nil {
		out.ModelStats = map[string]*ModelStats{}
	}
	if out.Records == nil {
		out.Records = make([]*UsageRecord, 0, 128)
	}
	return &out
}

func newHistoricalData() *HistoricalData {
	return &HistoricalData{
		DailyStats:   map[string]*DailyStats{},
		WeeklyStats:  map[string]*WeeklyStats{},
		MonthlyStats: map[string]*MonthlyStats{},
		ModelStats:   map[string]*ModelStats{},
		Records:      make([]*UsageRecord, 0, 128),
	}
}
