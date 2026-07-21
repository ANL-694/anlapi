package service

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"anlapi/internal/config"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/robfig/cron/v3"
)

const (
	opsScheduledReportJobName = "ops_scheduled_reports"

	opsScheduledReportLeaderLockKeyDefault = "ops:scheduled_reports:leader"
	opsScheduledReportLeaderLockTTLDefault = 5 * time.Minute

	opsScheduledReportLastRunKeyPrefix = "ops:scheduled_reports:last_run:"

	opsScheduledReportTickInterval = 1 * time.Minute
)

var opsScheduledReportCronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

var opsScheduledReportReleaseScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("DEL", KEYS[1])
end
return 0
`)

type OpsScheduledReportService struct {
	opsService   *OpsService
	userService  *UserService
	emailService *EmailService
	redisClient  *redis.Client
	cfg          *config.Config

	instanceID string
	loc        *time.Location

	distributedLockOn bool
	warnNoRedisOnce   sync.Once

	startOnce sync.Once
	stopOnce  sync.Once
	stopCtx   context.Context
	stop      context.CancelFunc
	wg        sync.WaitGroup
}

func NewOpsScheduledReportService(
	opsService *OpsService,
	userService *UserService,
	emailService *EmailService,
	redisClient *redis.Client,
	cfg *config.Config,
) *OpsScheduledReportService {
	lockOn := cfg == nil || strings.TrimSpace(cfg.RunMode) != config.RunModeSimple

	loc := time.Local
	if cfg != nil && strings.TrimSpace(cfg.Timezone) != "" {
		if parsed, err := time.LoadLocation(strings.TrimSpace(cfg.Timezone)); err == nil && parsed != nil {
			loc = parsed
		}
	}
	return &OpsScheduledReportService{
		opsService:   opsService,
		userService:  userService,
		emailService: emailService,
		redisClient:  redisClient,
		cfg:          cfg,

		instanceID:        uuid.NewString(),
		loc:               loc,
		distributedLockOn: lockOn,
		warnNoRedisOnce:   sync.Once{},
		startOnce:         sync.Once{},
		stopOnce:          sync.Once{},
		stopCtx:           nil,
		stop:              nil,
		wg:                sync.WaitGroup{},
	}
}

func (s *OpsScheduledReportService) Start() {
	s.StartWithContext(context.Background())
}

func (s *OpsScheduledReportService) StartWithContext(ctx context.Context) {
	if s == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if s.cfg != nil && !s.cfg.Ops.Enabled {
		return
	}
	if s.opsService == nil || s.emailService == nil {
		return
	}

	s.startOnce.Do(func() {
		s.stopCtx, s.stop = context.WithCancel(ctx)
		s.wg.Add(1)
		go s.run()
	})
}

func (s *OpsScheduledReportService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		if s.stop != nil {
			s.stop()
		}
	})
	s.wg.Wait()
}

func (s *OpsScheduledReportService) run() {
	defer s.wg.Done()

	ticker := time.NewTicker(opsScheduledReportTickInterval)
	defer ticker.Stop()

	s.runOnce()
	for {
		select {
		case <-ticker.C:
			s.runOnce()
		case <-s.stopCtx.Done():
			return
		}
	}
}

func (s *OpsScheduledReportService) runOnce() {
	if s == nil || s.opsService == nil || s.emailService == nil {
		return
	}

	startedAt := time.Now().UTC()
	runAt := startedAt

	ctx, cancel := context.WithTimeout(s.stopCtx, 60*time.Second)
	defer cancel()

	// Respect ops monitoring enabled switch.
	if !s.opsService.IsMonitoringEnabled(ctx) {
		return
	}

	release, ok := s.tryAcquireLeaderLock(ctx)
	if !ok {
		return
	}
	if release != nil {
		defer release()
	}

	now := time.Now()
	if s.loc != nil {
		now = now.In(s.loc)
	}

	reports := s.listScheduledReports(ctx, now)
	if len(reports) == 0 {
		return
	}

	reportsTotal := len(reports)
	reportsDue := 0
	sentAttempts := 0

	for _, report := range reports {
		if report == nil || !report.Enabled {
			continue
		}
		if report.NextRunAt.After(now) {
			continue
		}
		reportsDue++

		attempts, err := s.runReport(ctx, report, now)
		if err != nil {
			s.recordHeartbeatError(runAt, time.Since(startedAt), err)
			return
		}
		sentAttempts += attempts
	}

	result := truncateString(fmt.Sprintf("reports=%d due=%d send_attempts=%d", reportsTotal, reportsDue, sentAttempts), 2048)
	s.recordHeartbeatSuccess(runAt, time.Since(startedAt), result)
}

type opsScheduledReport struct {
	Name       string
	ReportType string
	Schedule   string
	Enabled    bool

	TimeRange time.Duration

	Recipients []string

	ErrorDigestMinCount             int
	AccountHealthErrorRateThreshold float64

	LastRunAt *time.Time
	NextRunAt time.Time
}

func (s *OpsScheduledReportService) listScheduledReports(ctx context.Context, now time.Time) []*opsScheduledReport {
	if s == nil || s.opsService == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	emailCfg, err := s.opsService.GetEmailNotificationConfig(ctx)
	if err != nil || emailCfg == nil {
		return nil
	}
	if !emailCfg.Report.Enabled {
		return nil
	}

	recipients := normalizeEmails(emailCfg.Report.Recipients)

	type reportDef struct {
		enabled   bool
		name      string
		kind      string
		timeRange time.Duration
		schedule  string
	}

	defs := []reportDef{
		{enabled: emailCfg.Report.DailySummaryEnabled, name: "日报", kind: "daily_summary", timeRange: 24 * time.Hour, schedule: emailCfg.Report.DailySummarySchedule},
		{enabled: emailCfg.Report.WeeklySummaryEnabled, name: "周报", kind: "weekly_summary", timeRange: 7 * 24 * time.Hour, schedule: emailCfg.Report.WeeklySummarySchedule},
		{enabled: emailCfg.Report.ErrorDigestEnabled, name: "错误摘要", kind: "error_digest", timeRange: 24 * time.Hour, schedule: emailCfg.Report.ErrorDigestSchedule},
		{enabled: emailCfg.Report.AccountHealthEnabled, name: "账号健康", kind: "account_health", timeRange: 24 * time.Hour, schedule: emailCfg.Report.AccountHealthSchedule},
	}

	out := make([]*opsScheduledReport, 0, len(defs))
	for _, d := range defs {
		if !d.enabled {
			continue
		}
		spec := strings.TrimSpace(d.schedule)
		if spec == "" {
			continue
		}
		sched, err := opsScheduledReportCronParser.Parse(spec)
		if err != nil {
			log.Printf("[OpsScheduledReport] invalid cron spec=%q for report=%s: %v", spec, d.kind, err)
			continue
		}

		lastRun := s.getLastRunAt(ctx, d.kind)
		base := lastRun
		if base.IsZero() {
			// Allow a schedule matching the current minute to trigger right after startup.
			base = now.Add(-1 * time.Minute)
		}
		next := sched.Next(base)
		if next.IsZero() {
			continue
		}

		var lastRunPtr *time.Time
		if !lastRun.IsZero() {
			lastCopy := lastRun
			lastRunPtr = &lastCopy
		}

		out = append(out, &opsScheduledReport{
			Name:       d.name,
			ReportType: d.kind,
			Schedule:   spec,
			Enabled:    true,

			TimeRange: d.timeRange,

			Recipients: recipients,

			ErrorDigestMinCount:             emailCfg.Report.ErrorDigestMinCount,
			AccountHealthErrorRateThreshold: emailCfg.Report.AccountHealthErrorRateThreshold,

			LastRunAt: lastRunPtr,
			NextRunAt: next,
		})
	}

	return out
}

func (s *OpsScheduledReportService) runReport(ctx context.Context, report *opsScheduledReport, now time.Time) (int, error) {
	if s == nil || s.opsService == nil || s.emailService == nil || report == nil {
		return 0, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	// Mark as "run" up-front so a broken SMTP config doesn't spam retries every minute.
	s.setLastRunAt(ctx, report.ReportType, now)

	content, err := s.generateReportHTML(ctx, report, now)
	if err != nil {
		return 0, err
	}
	if strings.TrimSpace(content) == "" {
		// Skip sending when the report decides not to emit content (e.g., digest below min count).
		return 0, nil
	}

	recipients := report.Recipients
	if len(recipients) == 0 && s.userService != nil {
		admin, err := s.userService.GetFirstAdmin(ctx)
		if err == nil && admin != nil && strings.TrimSpace(admin.Email) != "" {
			recipients = []string{strings.TrimSpace(admin.Email)}
		}
	}
	if len(recipients) == 0 {
		return 0, nil
	}

	subject := fmt.Sprintf("[ANL 运维报表] %s", strings.TrimSpace(report.Name))

	attempts := 0
	for _, to := range recipients {
		addr := strings.TrimSpace(to)
		if addr == "" {
			continue
		}
		attempts++
		if err := s.emailService.SendEmail(ctx, addr, subject, content); err != nil {
			// Ignore per-recipient failures; continue best-effort.
			continue
		}
	}
	return attempts, nil
}

func (s *OpsScheduledReportService) generateReportHTML(ctx context.Context, report *opsScheduledReport, now time.Time) (string, error) {
	if s == nil || s.opsService == nil || report == nil {
		return "", fmt.Errorf("service not initialized")
	}
	if report.TimeRange <= 0 {
		return "", fmt.Errorf("invalid time range")
	}

	end := now.UTC()
	start := end.Add(-report.TimeRange)

	switch strings.TrimSpace(report.ReportType) {
	case "daily_summary", "weekly_summary":
		overview, err := s.opsService.GetDashboardOverview(ctx, &OpsDashboardFilter{
			StartTime: start,
			EndTime:   end,
			Platform:  "",
			GroupID:   nil,
			QueryMode: OpsQueryModeAuto,
		})
		if err != nil {
			// If pre-aggregation isn't ready but the report is requested, fall back to raw.
			if strings.TrimSpace(report.ReportType) == "daily_summary" || strings.TrimSpace(report.ReportType) == "weekly_summary" {
				overview, err = s.opsService.GetDashboardOverview(ctx, &OpsDashboardFilter{
					StartTime: start,
					EndTime:   end,
					Platform:  "",
					GroupID:   nil,
					QueryMode: OpsQueryModeRaw,
				})
			}
			if err != nil {
				return "", err
			}
		}
		return buildOpsSummaryEmailHTML(report.Name, start, end, overview), nil
	case "error_digest":
		// Lightweight digest: list recent errors (status>=400) and breakdown by type.
		startTime := start
		endTime := end
		filter := &OpsErrorLogFilter{
			StartTime: &startTime,
			EndTime:   &endTime,
			Page:      1,
			PageSize:  100,
		}
		out, err := s.opsService.GetErrorLogs(ctx, filter)
		if err != nil {
			return "", err
		}
		if report.ErrorDigestMinCount > 0 && out != nil && out.Total < report.ErrorDigestMinCount {
			return "", nil
		}
		return buildOpsErrorDigestEmailHTML(report.Name, start, end, out), nil
	case "account_health":
		// Best-effort: use account availability (not error rate yet).
		avail, err := s.opsService.GetAccountAvailability(ctx, "", nil)
		if err != nil {
			return "", err
		}
		_ = report.AccountHealthErrorRateThreshold // reserved for future per-account error rate report
		return buildOpsAccountHealthEmailHTML(report.Name, start, end, avail), nil
	default:
		return "", fmt.Errorf("unknown report type: %s", report.ReportType)
	}
}

func buildOpsSummaryEmailHTML(title string, start, end time.Time, overview *OpsDashboardOverview) string {
	if overview == nil {
		return buildOpsReportEmailShell(title, start, end, `<p style="margin:0;color:#64748b;">当前周期暂无可用数据。</p>`)
	}

	latP50 := "-"
	latP99 := "-"
	if overview.Duration.P50 != nil {
		latP50 = fmt.Sprintf("%dms", *overview.Duration.P50)
	}
	if overview.Duration.P99 != nil {
		latP99 = fmt.Sprintf("%dms", *overview.Duration.P99)
	}

	ttftP50 := "-"
	ttftP99 := "-"
	if overview.TTFT.P50 != nil {
		ttftP50 = fmt.Sprintf("%dms", *overview.TTFT.P50)
	}
	if overview.TTFT.P99 != nil {
		ttftP99 = fmt.Sprintf("%dms", *overview.TTFT.P99)
	}

	body := fmt.Sprintf(`
<table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="border-collapse:collapse;margin-bottom:20px;">
  <tr>
    <td width="33%%" style="padding:12px;background:#f8fafc;border:1px solid #e2e8f0;"><div style="font-size:12px;color:#64748b;">总请求</div><div style="font-size:24px;font-weight:700;color:#0f172a;">%d</div></td>
    <td width="33%%" style="padding:12px;background:#f0fdf4;border:1px solid #bbf7d0;"><div style="font-size:12px;color:#166534;">成功</div><div style="font-size:24px;font-weight:700;color:#166534;">%d</div></td>
    <td width="34%%" style="padding:12px;background:#fef2f2;border:1px solid #fecaca;"><div style="font-size:12px;color:#991b1b;">SLA 错误</div><div style="font-size:24px;font-weight:700;color:#991b1b;">%d</div></td>
  </tr>
</table>
<table role="presentation" width="100%%" cellpadding="8" cellspacing="0" style="border-collapse:collapse;font-size:14px;color:#334155;">
  <tr><td style="border-bottom:1px solid #e2e8f0;">业务受限</td><td align="right" style="border-bottom:1px solid #e2e8f0;font-weight:600;">%d</td></tr>
  <tr><td style="border-bottom:1px solid #e2e8f0;">SLA / 错误率</td><td align="right" style="border-bottom:1px solid #e2e8f0;font-weight:600;">%.2f%% / %.2f%%</td></tr>
  <tr><td style="border-bottom:1px solid #e2e8f0;">上游错误率（不含 429/529）</td><td align="right" style="border-bottom:1px solid #e2e8f0;font-weight:600;">%.2f%%</td></tr>
  <tr><td style="border-bottom:1px solid #e2e8f0;">上游错误</td><td align="right" style="border-bottom:1px solid #e2e8f0;font-weight:600;">其他 %d / 429 %d / 529 %d</td></tr>
  <tr><td style="border-bottom:1px solid #e2e8f0;">请求延迟 p50 / p99</td><td align="right" style="border-bottom:1px solid #e2e8f0;font-weight:600;">%s / %s</td></tr>
  <tr><td style="border-bottom:1px solid #e2e8f0;">首 Token 延迟 p50 / p99</td><td align="right" style="border-bottom:1px solid #e2e8f0;font-weight:600;">%s / %s</td></tr>
  <tr><td style="border-bottom:1px solid #e2e8f0;">Token 消耗</td><td align="right" style="border-bottom:1px solid #e2e8f0;font-weight:600;">%d</td></tr>
  <tr><td style="border-bottom:1px solid #e2e8f0;">QPS 当前 / 峰值 / 平均</td><td align="right" style="border-bottom:1px solid #e2e8f0;font-weight:600;">%.1f / %.1f / %.1f</td></tr>
  <tr><td>TPS 当前 / 峰值 / 平均</td><td align="right" style="font-weight:600;">%.1f / %.1f / %.1f</td></tr>
</table>`,
		overview.RequestCountTotal,
		overview.SuccessCount,
		overview.ErrorCountSLA,
		overview.BusinessLimitedCount,
		overview.SLA*100,
		overview.ErrorRate*100,
		overview.UpstreamErrorRate*100,
		overview.UpstreamErrorCountExcl429529,
		overview.Upstream429Count,
		overview.Upstream529Count,
		htmlEscape(latP50),
		htmlEscape(latP99),
		htmlEscape(ttftP50),
		htmlEscape(ttftP99),
		overview.TokenConsumed,
		overview.QPS.Current,
		overview.QPS.Peak,
		overview.QPS.Avg,
		overview.TPS.Current,
		overview.TPS.Peak,
		overview.TPS.Avg,
	)
	return buildOpsReportEmailShell(title, start, end, body)
}

func buildOpsErrorDigestEmailHTML(title string, start, end time.Time, list *OpsErrorLogList) string {
	total := 0
	recent := []*OpsErrorLog{}
	if list != nil {
		total = list.Total
		recent = list.Errors
	}
	if len(recent) > 10 {
		recent = recent[:10]
	}

	rows := ""
	for _, item := range recent {
		if item == nil {
			continue
		}
		rows += fmt.Sprintf(
			"<tr><td style=\"padding:8px;border-bottom:1px solid #e2e8f0;white-space:nowrap;\">%s</td><td style=\"padding:8px;border-bottom:1px solid #e2e8f0;\">%s</td><td style=\"padding:8px;border-bottom:1px solid #e2e8f0;\">%d</td><td style=\"padding:8px;border-bottom:1px solid #e2e8f0;word-break:break-word;\">%s</td></tr>",
			htmlEscape(item.CreatedAt.UTC().Format(time.RFC3339)),
			htmlEscape(item.Platform),
			item.StatusCode,
			htmlEscape(truncateString(item.Message, 180)),
		)
	}
	if rows == "" {
		rows = "<tr><td colspan=\"4\" style=\"padding:16px;color:#64748b;\">当前周期没有错误记录。</td></tr>"
	}

	body := fmt.Sprintf(`
<p style="margin:0 0 16px;color:#334155;">错误总数：<strong style="color:#b91c1c;">%d</strong></p>
<div style="overflow-x:auto;">
<table width="100%%" cellpadding="0" cellspacing="0" style="border-collapse:collapse;font-size:13px;color:#334155;">
  <thead><tr style="background:#f8fafc;"><th align="left" style="padding:8px;">时间</th><th align="left" style="padding:8px;">平台</th><th align="left" style="padding:8px;">状态</th><th align="left" style="padding:8px;">消息</th></tr></thead>
  <tbody>%s</tbody>
</table>
</div>`, total, rows)
	return buildOpsReportEmailShell(title, start, end, body)
}

func buildOpsAccountHealthEmailHTML(title string, start, end time.Time, avail *OpsAccountAvailability) string {
	total := 0
	available := 0
	rateLimited := 0
	hasError := 0

	if avail != nil && avail.Accounts != nil {
		for _, a := range avail.Accounts {
			if a == nil {
				continue
			}
			total++
			if a.IsAvailable {
				available++
			}
			if a.IsRateLimited {
				rateLimited++
			}
			if a.HasError {
				hasError++
			}
		}
	}

	body := fmt.Sprintf(`
<table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="border-collapse:collapse;">
  <tr><td style="padding:10px;border-bottom:1px solid #e2e8f0;">账号总数</td><td align="right" style="padding:10px;border-bottom:1px solid #e2e8f0;font-weight:700;">%d</td></tr>
  <tr><td style="padding:10px;border-bottom:1px solid #e2e8f0;color:#166534;">可用</td><td align="right" style="padding:10px;border-bottom:1px solid #e2e8f0;font-weight:700;color:#166534;">%d</td></tr>
  <tr><td style="padding:10px;border-bottom:1px solid #e2e8f0;color:#92400e;">限流</td><td align="right" style="padding:10px;border-bottom:1px solid #e2e8f0;font-weight:700;color:#92400e;">%d</td></tr>
  <tr><td style="padding:10px;color:#991b1b;">异常</td><td align="right" style="padding:10px;font-weight:700;color:#991b1b;">%d</td></tr>
</table>
<p style="margin:16px 0 0;color:#64748b;font-size:12px;">当前报表反映账号可用性状态，不代表完整的账号错误率。</p>`, total, available, rateLimited, hasError)
	return buildOpsReportEmailShell(title, start, end, body)
}

func buildOpsReportEmailShell(title string, start, end time.Time, body string) string {
	return fmt.Sprintf(`<!doctype html>
<html><body style="margin:0;background:#f1f5f9;font-family:Arial,'Microsoft YaHei',sans-serif;color:#0f172a;">
<table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="background:#f1f5f9;padding:24px 12px;"><tr><td align="center">
  <table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="max-width:680px;background:#ffffff;border:1px solid #e2e8f0;">
    <tr><td style="padding:22px 24px;background:#0f172a;color:#ffffff;"><div style="font-size:12px;color:#94a3b8;">ANL API 运维中心</div><h1 style="margin:6px 0 0;font-size:22px;letter-spacing:0;">%s</h1></td></tr>
    <tr><td style="padding:14px 24px;border-bottom:1px solid #e2e8f0;color:#64748b;font-size:12px;">统计周期：%s 至 %s（UTC）</td></tr>
    <tr><td style="padding:24px;">%s</td></tr>
    <tr><td style="padding:14px 24px;border-top:1px solid #e2e8f0;color:#94a3b8;font-size:11px;">此邮件由 ANL API 自动生成。</td></tr>
  </table>
</td></tr></table>
</body></html>`,
		htmlEscape(strings.TrimSpace(title)),
		htmlEscape(start.UTC().Format("2006-01-02 15:04")),
		htmlEscape(end.UTC().Format("2006-01-02 15:04")),
		body,
	)
}

func (s *OpsScheduledReportService) tryAcquireLeaderLock(ctx context.Context) (func(), bool) {
	if s == nil || !s.distributedLockOn {
		return nil, true
	}
	if s.redisClient == nil {
		s.warnNoRedisOnce.Do(func() {
			log.Printf("[OpsScheduledReport] redis not configured; running without distributed lock")
		})
		return nil, true
	}
	if ctx == nil {
		ctx = context.Background()
	}

	key := opsScheduledReportLeaderLockKeyDefault
	ttl := opsScheduledReportLeaderLockTTLDefault
	if strings.TrimSpace(key) == "" {
		key = "ops:scheduled_reports:leader"
	}
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}

	ok, err := s.redisClient.SetNX(ctx, key, s.instanceID, ttl).Result()
	if err != nil {
		// Prefer fail-closed to avoid duplicate report sends when Redis is flaky.
		log.Printf("[OpsScheduledReport] leader lock SetNX failed; skipping this cycle: %v", err)
		return nil, false
	}
	if !ok {
		return nil, false
	}
	return func() {
		_, _ = opsScheduledReportReleaseScript.Run(ctx, s.redisClient, []string{key}, s.instanceID).Result()
	}, true
}

func (s *OpsScheduledReportService) getLastRunAt(ctx context.Context, reportType string) time.Time {
	if s == nil || s.redisClient == nil {
		return time.Time{}
	}
	kind := strings.TrimSpace(reportType)
	if kind == "" {
		return time.Time{}
	}
	key := opsScheduledReportLastRunKeyPrefix + kind

	raw, err := s.redisClient.Get(ctx, key).Result()
	if err != nil || strings.TrimSpace(raw) == "" {
		return time.Time{}
	}
	sec, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || sec <= 0 {
		return time.Time{}
	}
	last := time.Unix(sec, 0)
	// Cron schedules are interpreted in the configured timezone (s.loc). Ensure the base time
	// passed into cron.Next() uses the same location; otherwise the job will drift by timezone
	// offset (e.g. Asia/Shanghai default would run 8h later after the first execution).
	if s.loc != nil {
		return last.In(s.loc)
	}
	return last.UTC()
}

func (s *OpsScheduledReportService) setLastRunAt(ctx context.Context, reportType string, t time.Time) {
	if s == nil || s.redisClient == nil {
		return
	}
	kind := strings.TrimSpace(reportType)
	if kind == "" {
		return
	}
	if t.IsZero() {
		t = time.Now().UTC()
	}
	key := opsScheduledReportLastRunKeyPrefix + kind
	_ = s.redisClient.Set(ctx, key, strconv.FormatInt(t.UTC().Unix(), 10), 14*24*time.Hour).Err()
}

func (s *OpsScheduledReportService) recordHeartbeatSuccess(runAt time.Time, duration time.Duration, result string) {
	if s == nil || s.opsService == nil || s.opsService.opsRepo == nil {
		return
	}
	now := time.Now().UTC()
	durMs := duration.Milliseconds()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	msg := strings.TrimSpace(result)
	if msg == "" {
		msg = "ok"
	}
	msg = truncateString(msg, 2048)
	_ = s.opsService.opsRepo.UpsertJobHeartbeat(ctx, &OpsUpsertJobHeartbeatInput{
		JobName:        opsScheduledReportJobName,
		LastRunAt:      &runAt,
		LastSuccessAt:  &now,
		LastDurationMs: &durMs,
		LastResult:     &msg,
	})
}

func (s *OpsScheduledReportService) recordHeartbeatError(runAt time.Time, duration time.Duration, err error) {
	if s == nil || s.opsService == nil || s.opsService.opsRepo == nil || err == nil {
		return
	}
	now := time.Now().UTC()
	durMs := duration.Milliseconds()
	msg := truncateString(err.Error(), 2048)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = s.opsService.opsRepo.UpsertJobHeartbeat(ctx, &OpsUpsertJobHeartbeatInput{
		JobName:        opsScheduledReportJobName,
		LastRunAt:      &runAt,
		LastErrorAt:    &now,
		LastError:      &msg,
		LastDurationMs: &durMs,
	})
}

func normalizeEmails(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, raw := range in {
		addr := strings.ToLower(strings.TrimSpace(raw))
		if addr == "" {
			continue
		}
		if _, ok := seen[addr]; ok {
			continue
		}
		seen[addr] = struct{}{}
		out = append(out, addr)
	}
	return out
}
