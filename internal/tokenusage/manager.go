package tokenusage

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"
)

type noopNotifier struct{}

func (noopNotifier) Notify(context.Context, *Alert) error { return nil }

// NewTokenUsageManager 创建Token使用量管理器。
func NewTokenUsageManager(cfg *Config) (*TokenUsageManager, error) {
	config := DefaultConfig()
	if cfg != nil {
		config = *cfg
	}
	if err := normalizeConfig(&config); err != nil {
		return nil, err
	}

	storage, err := createStorage(config)
	if err != nil {
		// 优雅降级：回退到内存存储，避免主流程不可用。
		storage = NewMemoryStorage()
	}

	historical, loadErr := storage.LoadHistorical()
	if loadErr != nil {
		historical = newHistoricalData()
	}

	mgr := &TokenUsageManager{
		config:         config,
		storage:        storage,
		metrics:        &Metrics{},
		realtimeStats:  map[string]*SessionStats{},
		historicalData: cloneHistoricalData(historical),
		notifier:       noopNotifier{},
		logger:         slog.Default().With("component", "token_usage"),
		dirtySessions:  map[string]struct{}{},
		tpsSamples:     make([]tpsSample, 0, 64),
		stopCh:         make(chan struct{}),
	}
	if err != nil {
		mgr.logger.Warn("storage init failed, fallback to memory", "error", err)
	}
	if loadErr != nil {
		mgr.logger.Warn("historical load failed, using empty history", "error", loadErr)
	}
	if sessions, listErr := storage.ListSessions(time.Time{}, time.Time{}); listErr == nil {
		for _, stats := range sessions {
			mgr.realtimeStats[stats.SessionID] = cloneSessionStats(stats)
		}
		mgr.metrics.ActiveSessions = len(mgr.realtimeStats)
	}

	mgr.wg.Add(1)
	go mgr.persistenceLoop()
	return mgr, nil
}

func normalizeConfig(cfg *Config) error {
	if cfg == nil {
		return wrapError(ErrCodeInvalidConfig, "config is nil", nil)
	}
	cfg.StorageType = strings.TrimSpace(strings.ToLower(cfg.StorageType))
	if cfg.StorageType == "" {
		cfg.StorageType = "memory"
	}
	switch cfg.StorageType {
	case "memory", "file", "database":
	default:
		return wrapError(ErrCodeInvalidConfig, "storage_type must be memory/file/database", nil)
	}
	if cfg.BackupInterval <= 0 {
		cfg.BackupInterval = time.Minute
	}
	if cfg.RetentionDays <= 0 {
		cfg.RetentionDays = 30
	}
	if cfg.MaxSessions <= 0 {
		cfg.MaxSessions = 10000
	}
	if cfg.MonitorInterval <= 0 {
		cfg.MonitorInterval = 30 * time.Second
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 200
	}
	if strings.TrimSpace(cfg.DatabaseDriver) == "" {
		cfg.DatabaseDriver = "sqlite3"
	}
	return nil
}

func createStorage(cfg Config) (Storage, error) {
	switch cfg.StorageType {
	case "memory":
		return NewMemoryStorage(), nil
	case "file":
		return NewFileStorage(cfg.StoragePath)
	case "database":
		return NewDatabaseStorage(cfg.DatabaseDriver, cfg.StoragePath)
	default:
		return NewMemoryStorage(), nil
	}
}

func (tm *TokenUsageManager) SetNotifier(notifier Notifier) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if notifier == nil {
		tm.notifier = noopNotifier{}
		return
	}
	tm.notifier = notifier
}

// RecordTokenUsage 记录token使用。
func (tm *TokenUsageManager) RecordTokenUsage(ctx context.Context, req *TokenRecordRequest) error {
	if err := ctx.Err(); err != nil {
		return wrapError(ErrCodeTimeout, "context canceled", err)
	}
	if req == nil {
		return wrapError(ErrCodeInvalidInput, "request is nil", nil)
	}
	if strings.TrimSpace(req.SessionID) == "" {
		return wrapError(ErrCodeInvalidInput, "session_id is required", nil)
	}
	if req.InputTokens < 0 || req.OutputTokens < 0 {
		return wrapError(ErrCodeInvalidInput, "token count must be non-negative", nil)
	}

	now := time.Now().UTC()
	total := req.InputTokens + req.OutputTokens
	sessionID := strings.TrimSpace(req.SessionID)
	model := strings.TrimSpace(req.ModelName)
	if model == "" {
		model = "unknown"
	}

	record := &UsageRecord{
		Time:         now,
		SessionID:    sessionID,
		ModelName:    model,
		InputTokens:  req.InputTokens,
		OutputTokens: req.OutputTokens,
		TotalTokens:  total,
		RequestID:    strings.TrimSpace(req.RequestID),
		Latency:      req.Latency,
		Success:      req.Success,
		Metadata:     cloneMap(req.Metadata),
	}

	tm.mu.Lock()
	defer tm.mu.Unlock()

	stats, ok := tm.realtimeStats[sessionID]
	if !ok {
		stats = &SessionStats{
			SessionID: sessionID,
			StartTime: now,
			Metadata:  map[string]string{},
		}
		tm.realtimeStats[sessionID] = stats
	}
	stats.LastUpdate = now
	stats.InputTokens += req.InputTokens
	stats.OutputTokens += req.OutputTokens
	stats.TotalTokens += total
	stats.RequestCount++
	if stats.RequestCount > 0 {
		stats.AverageTokens = float64(stats.TotalTokens) / float64(stats.RequestCount)
	}
	if stats.Metadata == nil {
		stats.Metadata = map[string]string{}
	}
	for k, v := range req.Metadata {
		stats.Metadata[k] = v
	}

	tm.historicalData.Records = append(tm.historicalData.Records, record)
	tm.updateHistoricalLocked(record)
	tm.updateMetricsLocked(record)

	tm.dirtySessions[sessionID] = struct{}{}
	tm.dirtyHistorical = true
	tm.metrics.ActiveSessions = len(tm.realtimeStats)

	tm.pruneSessionsLocked()
	tm.cleanupRecordsLocked(now)

	return nil
}

func (tm *TokenUsageManager) updateHistoricalLocked(record *UsageRecord) {
	dayKey := record.Time.Format("2006-01-02")
	year, week := record.Time.ISOWeek()
	weekKey := fmt.Sprintf("%04d-W%02d", year, week)
	monthKey := record.Time.Format("2006-01")

	day := tm.historicalData.DailyStats[dayKey]
	if day == nil {
		day = &DailyStats{Date: dayKey, ModelTokens: map[string]int64{}}
		tm.historicalData.DailyStats[dayKey] = day
	}
	applyDaily(day, record)

	weekStats := tm.historicalData.WeeklyStats[weekKey]
	if weekStats == nil {
		weekStats = &WeeklyStats{Week: weekKey, ModelTokens: map[string]int64{}}
		tm.historicalData.WeeklyStats[weekKey] = weekStats
	}
	applyWeekly(weekStats, record)

	month := tm.historicalData.MonthlyStats[monthKey]
	if month == nil {
		month = &MonthlyStats{Month: monthKey, ModelTokens: map[string]int64{}}
		tm.historicalData.MonthlyStats[monthKey] = month
	}
	applyMonthly(month, record)

	modelStats := tm.historicalData.ModelStats[record.ModelName]
	if modelStats == nil {
		modelStats = &ModelStats{ModelName: record.ModelName}
		tm.historicalData.ModelStats[record.ModelName] = modelStats
	}
	applyModel(modelStats, record)
}

func applyDaily(stats *DailyStats, record *UsageRecord) {
	stats.InputTokens += record.InputTokens
	stats.OutputTokens += record.OutputTokens
	stats.TotalTokens += record.TotalTokens
	stats.RequestCount++
	if record.Success {
		stats.SuccessCount++
	} else {
		stats.ErrorCount++
	}
	stats.AvgLatencyMs = runningAverage(stats.AvgLatencyMs, stats.RequestCount, float64(record.Latency.Milliseconds()))
	if stats.ModelTokens == nil {
		stats.ModelTokens = map[string]int64{}
	}
	stats.ModelTokens[record.ModelName] += record.TotalTokens
}

func applyWeekly(stats *WeeklyStats, record *UsageRecord) {
	stats.InputTokens += record.InputTokens
	stats.OutputTokens += record.OutputTokens
	stats.TotalTokens += record.TotalTokens
	stats.RequestCount++
	if record.Success {
		stats.SuccessCount++
	} else {
		stats.ErrorCount++
	}
	stats.AvgLatencyMs = runningAverage(stats.AvgLatencyMs, stats.RequestCount, float64(record.Latency.Milliseconds()))
	if stats.ModelTokens == nil {
		stats.ModelTokens = map[string]int64{}
	}
	stats.ModelTokens[record.ModelName] += record.TotalTokens
}

func applyMonthly(stats *MonthlyStats, record *UsageRecord) {
	stats.InputTokens += record.InputTokens
	stats.OutputTokens += record.OutputTokens
	stats.TotalTokens += record.TotalTokens
	stats.RequestCount++
	if record.Success {
		stats.SuccessCount++
	} else {
		stats.ErrorCount++
	}
	stats.AvgLatencyMs = runningAverage(stats.AvgLatencyMs, stats.RequestCount, float64(record.Latency.Milliseconds()))
	if stats.ModelTokens == nil {
		stats.ModelTokens = map[string]int64{}
	}
	stats.ModelTokens[record.ModelName] += record.TotalTokens
}

func applyModel(stats *ModelStats, record *UsageRecord) {
	stats.InputTokens += record.InputTokens
	stats.OutputTokens += record.OutputTokens
	stats.TotalTokens += record.TotalTokens
	stats.RequestCount++
	if record.Success {
		stats.SuccessCount++
	} else {
		stats.ErrorCount++
	}
	stats.AvgLatencyMs = runningAverage(stats.AvgLatencyMs, stats.RequestCount, float64(record.Latency.Milliseconds()))
	stats.LastUpdated = record.Time
}

func runningAverage(current float64, count int64, next float64) float64 {
	if count <= 1 {
		return next
	}
	total := current*float64(count-1) + next
	return total / float64(count)
}

func (tm *TokenUsageManager) updateMetricsLocked(record *UsageRecord) {
	tm.totalRequests++
	if !record.Success {
		tm.totalErrors++
	}
	tm.totalLatency += record.Latency

	if tm.totalRequests > 0 {
		tm.metrics.ErrorRate = float64(tm.totalErrors) / float64(tm.totalRequests)
		tm.metrics.Latency = tm.totalLatency / time.Duration(tm.totalRequests)
	}

	now := record.Time
	tm.tpsSamples = append(tm.tpsSamples, tpsSample{Time: now, Tokens: record.TotalTokens})
	cutoff := now.Add(-1 * time.Second)
	start := 0
	var tokenSum int64
	for i := len(tm.tpsSamples) - 1; i >= 0; i-- {
		if tm.tpsSamples[i].Time.Before(cutoff) {
			start = i + 1
			break
		}
		tokenSum += tm.tpsSamples[i].Tokens
	}
	if start > 0 && start < len(tm.tpsSamples) {
		tm.tpsSamples = append([]tpsSample(nil), tm.tpsSamples[start:]...)
	}
	tm.metrics.CurrentTPS = float64(tokenSum)
	if tm.metrics.CurrentTPS > tm.metrics.PeakTPS {
		tm.metrics.PeakTPS = tm.metrics.CurrentTPS
	}
}

func (tm *TokenUsageManager) pruneSessionsLocked() {
	if tm.config.MaxSessions <= 0 || len(tm.realtimeStats) <= tm.config.MaxSessions {
		return
	}
	ids := make([]string, 0, len(tm.realtimeStats))
	for id := range tm.realtimeStats {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		return tm.realtimeStats[ids[i]].LastUpdate.Before(tm.realtimeStats[ids[j]].LastUpdate)
	})
	overflow := len(tm.realtimeStats) - tm.config.MaxSessions
	for i := 0; i < overflow; i++ {
		deleteID := ids[i]
		delete(tm.realtimeStats, deleteID)
		_ = tm.storage.DeleteSession(deleteID)
	}
}

func (tm *TokenUsageManager) cleanupRecordsLocked(now time.Time) {
	if tm.config.RetentionDays <= 0 || len(tm.historicalData.Records) == 0 {
		return
	}
	cutoff := now.AddDate(0, 0, -tm.config.RetentionDays)
	filtered := tm.historicalData.Records[:0]
	for _, rec := range tm.historicalData.Records {
		if rec.Time.Before(cutoff) {
			continue
		}
		filtered = append(filtered, rec)
	}
	tm.historicalData.Records = filtered
}

// GetRealtimeStats 获取实时统计。
func (tm *TokenUsageManager) GetRealtimeStats() (*RealtimeStats, error) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	sessions := make([]*SessionStats, 0, len(tm.realtimeStats))
	var total int64
	for _, stats := range tm.realtimeStats {
		snapshot := cloneSessionStats(stats)
		sessions = append(sessions, snapshot)
		total += snapshot.TotalTokens
	}
	return &RealtimeStats{
		Metrics:     *tm.metrics,
		Sessions:    sessions,
		GeneratedAt: time.Now().UTC(),
		TotalTokens: total,
	}, nil
}

// GetSessionStats 获取会话统计。
func (tm *TokenUsageManager) GetSessionStats(sessionID string) (*SessionStats, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, wrapError(ErrCodeInvalidInput, "session_id is required", nil)
	}
	tm.mu.RLock()
	if stats, ok := tm.realtimeStats[sessionID]; ok {
		out := cloneSessionStats(stats)
		tm.mu.RUnlock()
		return out, nil
	}
	tm.mu.RUnlock()

	stats, err := tm.storage.LoadSession(sessionID)
	if err != nil {
		return nil, err
	}
	return cloneSessionStats(stats), nil
}

// GetUsageByPeriod 获取时间段使用量。
func (tm *TokenUsageManager) GetUsageByPeriod(start, end time.Time, filter *UsageFilter) (*UsageReport, error) {
	if !start.IsZero() && !end.IsZero() && start.After(end) {
		return nil, wrapError(ErrCodeInvalidInput, "start time must be before end time", nil)
	}

	tm.mu.RLock()
	records := make([]*UsageRecord, 0, len(tm.historicalData.Records))
	for _, rec := range tm.historicalData.Records {
		records = append(records, rec)
	}
	tm.mu.RUnlock()

	report := &UsageReport{
		Type:         "custom",
		StartDate:    start,
		EndDate:      end,
		SessionStats: map[string]SessionStats{},
		ModelStats:   map[string]ModelStats{},
		DailyStats:   map[string]DailyStats{},
		GeneratedAt:  time.Now().UTC(),
	}
	var latencyTotal time.Duration
	for _, rec := range records {
		if !start.IsZero() && rec.Time.Before(start) {
			continue
		}
		if !end.IsZero() && rec.Time.After(end) {
			continue
		}
		if !matchesFilter(rec, filter) {
			continue
		}
		report.TotalInputTokens += rec.InputTokens
		report.TotalOutputTokens += rec.OutputTokens
		report.TotalTokens += rec.TotalTokens
		report.RequestCount++
		if rec.Success {
			report.SuccessCount++
		} else {
			report.ErrorCount++
		}
		latencyTotal += rec.Latency

		ss := report.SessionStats[rec.SessionID]
		ss.SessionID = rec.SessionID
		if ss.StartTime.IsZero() || rec.Time.Before(ss.StartTime) {
			ss.StartTime = rec.Time
		}
		if rec.Time.After(ss.LastUpdate) {
			ss.LastUpdate = rec.Time
		}
		ss.InputTokens += rec.InputTokens
		ss.OutputTokens += rec.OutputTokens
		ss.TotalTokens += rec.TotalTokens
		ss.RequestCount++
		if ss.RequestCount > 0 {
			ss.AverageTokens = float64(ss.TotalTokens) / float64(ss.RequestCount)
		}
		if ss.Metadata == nil {
			ss.Metadata = map[string]string{}
		}
		for k, v := range rec.Metadata {
			ss.Metadata[k] = v
		}
		report.SessionStats[rec.SessionID] = ss

		ms := report.ModelStats[rec.ModelName]
		ms.ModelName = rec.ModelName
		ms.InputTokens += rec.InputTokens
		ms.OutputTokens += rec.OutputTokens
		ms.TotalTokens += rec.TotalTokens
		ms.RequestCount++
		if rec.Success {
			ms.SuccessCount++
		} else {
			ms.ErrorCount++
		}
		ms.AvgLatencyMs = runningAverage(ms.AvgLatencyMs, ms.RequestCount, float64(rec.Latency.Milliseconds()))
		ms.LastUpdated = rec.Time
		report.ModelStats[rec.ModelName] = ms

		dayKey := rec.Time.Format("2006-01-02")
		ds := report.DailyStats[dayKey]
		ds.Date = dayKey
		ds.InputTokens += rec.InputTokens
		ds.OutputTokens += rec.OutputTokens
		ds.TotalTokens += rec.TotalTokens
		ds.RequestCount++
		if rec.Success {
			ds.SuccessCount++
		} else {
			ds.ErrorCount++
		}
		ds.AvgLatencyMs = runningAverage(ds.AvgLatencyMs, ds.RequestCount, float64(rec.Latency.Milliseconds()))
		if ds.ModelTokens == nil {
			ds.ModelTokens = map[string]int64{}
		}
		ds.ModelTokens[rec.ModelName] += rec.TotalTokens
		report.DailyStats[dayKey] = ds
	}
	if report.RequestCount > 0 {
		report.AvgLatency = latencyTotal / time.Duration(report.RequestCount)
	}
	return report, nil
}

func matchesFilter(rec *UsageRecord, filter *UsageFilter) bool {
	if filter == nil {
		return true
	}
	if len(filter.SessionIDs) > 0 {
		ok := false
		for _, id := range filter.SessionIDs {
			if rec.SessionID == id {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	if len(filter.ModelNames) > 0 {
		ok := false
		for _, name := range filter.ModelNames {
			if rec.ModelName == name {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	if filter.Success != nil && rec.Success != *filter.Success {
		return false
	}
	for k, v := range filter.Metadata {
		if rec.Metadata[k] != v {
			return false
		}
	}
	return true
}

// GetModelUsage 获取模型使用统计。
func (tm *TokenUsageManager) GetModelUsage(modelName string, period string) (*ModelUsageReport, error) {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return nil, wrapError(ErrCodeInvalidInput, "model name is required", nil)
	}
	start, end := periodRange(strings.TrimSpace(strings.ToLower(period)))
	report, err := tm.GetUsageByPeriod(start, end, &UsageFilter{ModelNames: []string{modelName}})
	if err != nil {
		return nil, err
	}
	stats, ok := report.ModelStats[modelName]
	if !ok {
		return &ModelUsageReport{
			ModelName: modelName,
			Period:    period,
			StartDate: start,
			EndDate:   end,
		}, nil
	}
	successRate := 0.0
	if stats.RequestCount > 0 {
		successRate = float64(stats.SuccessCount) / float64(stats.RequestCount)
	}
	avgTokens := 0.0
	if stats.RequestCount > 0 {
		avgTokens = float64(stats.TotalTokens) / float64(stats.RequestCount)
	}
	return &ModelUsageReport{
		ModelName:         modelName,
		Period:            period,
		TotalInputTokens:  stats.InputTokens,
		TotalOutputTokens: stats.OutputTokens,
		TotalTokens:       stats.TotalTokens,
		RequestCount:      stats.RequestCount,
		SuccessRate:       successRate,
		AverageTokens:     avgTokens,
		AvgLatency:        time.Duration(stats.AvgLatencyMs) * time.Millisecond,
		StartDate:         start,
		EndDate:           end,
	}, nil
}

func periodRange(period string) (time.Time, time.Time) {
	now := time.Now().UTC()
	switch period {
	case "daily", "day":
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		return start, now
	case "weekly", "week":
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -(weekday - 1))
		return start, now
	case "monthly", "month":
		start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		return start, now
	default:
		return time.Time{}, now
	}
}

// GenerateReport 生成使用报告。
func (tm *TokenUsageManager) GenerateReport(req *ReportRequest) (*UsageReport, error) {
	if req == nil {
		return nil, wrapError(ErrCodeInvalidInput, "report request is nil", nil)
	}
	reportType := strings.TrimSpace(strings.ToLower(req.Type))
	start, end := req.StartDate, req.EndDate
	switch reportType {
	case "", "custom":
		if end.IsZero() {
			end = time.Now().UTC()
		}
	case "daily", "day":
		start, end = periodRange("daily")
	case "weekly", "week":
		start, end = periodRange("weekly")
	case "monthly", "month":
		start, end = periodRange("monthly")
	default:
		return nil, wrapError(ErrCodeInvalidInput, "unsupported report type", nil)
	}
	report, err := tm.GetUsageByPeriod(start, end, req.Filters)
	if err != nil {
		return nil, err
	}
	report.Type = reportType
	if report.Type == "" {
		report.Type = "custom"
	}
	return report, nil
}

// ExportData 导出数据。
func (tm *TokenUsageManager) ExportData(req *ExportRequest) ([]byte, error) {
	if req == nil {
		return nil, wrapError(ErrCodeInvalidInput, "export request is nil", nil)
	}
	format := strings.TrimSpace(strings.ToLower(req.Format))
	if format == "" && req.Report != nil {
		format = strings.ToLower(strings.TrimSpace(req.Report.Format))
	}
	if format == "" {
		format = "json"
	}
	reportReq := req.Report
	if reportReq == nil {
		reportReq = &ReportRequest{Type: "daily"}
	}
	report, err := tm.GenerateReport(reportReq)
	if err != nil {
		return nil, err
	}

	switch format {
	case "json":
		return json.MarshalIndent(report, "", "  ")
	case "csv", "excel":
		// excel 优雅降级到CSV，便于直接下载/导入。
		var sb strings.Builder
		w := csv.NewWriter(&sb)
		_ = w.Write([]string{"type", report.Type})
		_ = w.Write([]string{"start", report.StartDate.Format(time.RFC3339)})
		_ = w.Write([]string{"end", report.EndDate.Format(time.RFC3339)})
		_ = w.Write([]string{"total_input_tokens", fmt.Sprint(report.TotalInputTokens)})
		_ = w.Write([]string{"total_output_tokens", fmt.Sprint(report.TotalOutputTokens)})
		_ = w.Write([]string{"total_tokens", fmt.Sprint(report.TotalTokens)})
		_ = w.Write([]string{"request_count", fmt.Sprint(report.RequestCount)})
		_ = w.Write([]string{"success_count", fmt.Sprint(report.SuccessCount)})
		_ = w.Write([]string{"error_count", fmt.Sprint(report.ErrorCount)})
		_ = w.Write([]string{})
		_ = w.Write([]string{"model", "input_tokens", "output_tokens", "total_tokens", "request_count", "success_count", "error_count"})

		modelNames := make([]string, 0, len(report.ModelStats))
		for name := range report.ModelStats {
			modelNames = append(modelNames, name)
		}
		sort.Strings(modelNames)
		for _, name := range modelNames {
			ms := report.ModelStats[name]
			_ = w.Write([]string{
				name,
				fmt.Sprint(ms.InputTokens),
				fmt.Sprint(ms.OutputTokens),
				fmt.Sprint(ms.TotalTokens),
				fmt.Sprint(ms.RequestCount),
				fmt.Sprint(ms.SuccessCount),
				fmt.Sprint(ms.ErrorCount),
			})
		}
		w.Flush()
		if err := w.Error(); err != nil {
			return nil, wrapError(ErrCodeInternal, "write export csv failed", err)
		}
		return []byte(sb.String()), nil
	default:
		return nil, wrapError(ErrCodeInvalidInput, "unsupported export format", nil)
	}
}

// StartMonitoring 启动监控。
func (tm *TokenUsageManager) StartMonitoring(ctx context.Context) error {
	if tm == nil || !tm.config.EnableRealtime {
		return nil
	}
	tm.monitorMu.Lock()
	if tm.monitorOn {
		tm.monitorMu.Unlock()
		return nil
	}
	tm.monitorOn = true
	tm.monitorMu.Unlock()

	tm.wg.Add(1)
	go func() {
		defer tm.wg.Done()
		ticker := time.NewTicker(tm.config.MonitorInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tm.stopCh:
				return
			case <-ticker.C:
				alertResult, err := tm.CheckThresholds()
				if err != nil {
					tm.logger.Warn("check thresholds failed", "error", err)
					continue
				}
				if len(alertResult.Alerts) == 0 {
					continue
				}
				tm.mu.RLock()
				notifier := tm.notifier
				tm.mu.RUnlock()
				for _, alert := range alertResult.Alerts {
					_ = notifier.Notify(ctx, alert)
				}
			}
		}
	}()
	return nil
}

// CheckThresholds 检查阈值。
func (tm *TokenUsageManager) CheckThresholds() (*AlertResult, error) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	now := time.Now().UTC()
	alerts := make([]*Alert, 0, 4)

	var total int64
	for _, s := range tm.realtimeStats {
		total += s.TotalTokens
	}
	if tm.config.AlertThreshold > 0 {
		ratio := float64(total) / float64(tm.config.AlertThreshold)
		if ratio >= 1.0 {
			alerts = append(alerts, &Alert{
				Type:    "usage_quota",
				Level:   "critical",
				Message: "token usage exceeded alert threshold",
				Value:   float64(total),
				Time:    now,
			})
		} else if ratio >= 0.8 {
			alerts = append(alerts, &Alert{
				Type:    "usage_quota",
				Level:   "warning",
				Message: "token usage approaching alert threshold",
				Value:   float64(total),
				Time:    now,
			})
		}
	}

	if tm.metrics.ErrorRate >= 0.25 {
		alerts = append(alerts, &Alert{
			Type:    "error_rate",
			Level:   "critical",
			Message: "error rate is above 25%",
			Value:   tm.metrics.ErrorRate,
			Time:    now,
		})
	} else if tm.metrics.ErrorRate >= 0.1 {
		alerts = append(alerts, &Alert{
			Type:    "error_rate",
			Level:   "warning",
			Message: "error rate is above 10%",
			Value:   tm.metrics.ErrorRate,
			Time:    now,
		})
	}

	latencyMs := float64(tm.metrics.Latency.Milliseconds())
	if tm.metrics.Latency >= 5*time.Second {
		alerts = append(alerts, &Alert{
			Type:    "latency",
			Level:   "critical",
			Message: "average latency is above 5s",
			Value:   latencyMs,
			Time:    now,
		})
	} else if tm.metrics.Latency >= 2*time.Second {
		alerts = append(alerts, &Alert{
			Type:    "latency",
			Level:   "warning",
			Message: "average latency is above 2s",
			Value:   latencyMs,
			Time:    now,
		})
	}
	return &AlertResult{Alerts: alerts}, nil
}

func (tm *TokenUsageManager) persistenceLoop() {
	defer tm.wg.Done()
	ticker := time.NewTicker(tm.config.BackupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-tm.stopCh:
			_ = tm.flush()
			return
		case <-ticker.C:
			_ = tm.flush()
		}
	}
}

func (tm *TokenUsageManager) flush() error {
	tm.mu.Lock()
	dirtySessions := make([]string, 0, len(tm.dirtySessions))
	for id := range tm.dirtySessions {
		dirtySessions = append(dirtySessions, id)
	}
	dirtyHistorical := tm.dirtyHistorical
	historicalSnapshot := cloneHistoricalData(tm.historicalData)
	sessionSnapshots := make(map[string]*SessionStats, len(dirtySessions))
	for _, id := range dirtySessions {
		sessionSnapshots[id] = cloneSessionStats(tm.realtimeStats[id])
	}
	tm.dirtySessions = map[string]struct{}{}
	tm.dirtyHistorical = false
	tm.mu.Unlock()

	for _, id := range dirtySessions {
		stats := sessionSnapshots[id]
		if stats == nil {
			continue
		}
		if err := tm.retry(func() error { return tm.storage.SaveSession(id, stats) }); err != nil {
			tm.logger.Warn("save session failed", "session_id", id, "error", err)
		}
	}
	if dirtyHistorical {
		if err := tm.retry(func() error { return tm.storage.SaveHistorical(historicalSnapshot) }); err != nil {
			tm.logger.Warn("save historical failed", "error", err)
		}
	}
	return nil
}

func (tm *TokenUsageManager) retry(fn func() error) error {
	var err error
	for i := 0; i < 3; i++ {
		err = fn()
		if err == nil {
			return nil
		}
		time.Sleep(time.Duration(i+1) * 50 * time.Millisecond)
	}
	return err
}

// Close 停止后台任务并落盘。
func (tm *TokenUsageManager) Close() error {
	if tm == nil {
		return nil
	}
	tm.stopOnce.Do(func() {
		close(tm.stopCh)
	})
	tm.wg.Wait()
	if closable, ok := tm.storage.(interface{ Close() error }); ok {
		return closable.Close()
	}
	return nil
}
