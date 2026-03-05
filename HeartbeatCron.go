package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/robfig/cron/v3"
)

// 心跳运行器
type HeartbeatRunner struct {
	workspace     string
	heartbeatPath string
	laneLock      *sync.Mutex
	interval      time.Duration
	activeHours   [2]int
	maxQueueSize  int
	lastRunAt     time.Time
	running       bool
	stopped       bool
	thread        chan struct{}
	outputQueue   []string
	queueLock     sync.Mutex
	lastOutput    string
	watcher       *fsnotify.Watcher // 文件系统监控器
}

// 定时任务
type CronJob struct {
	ID                string                 `json:"id"`
	Name              string                 `json:"name"`
	Enabled           bool                   `json:"enabled"`
	ScheduleKind      string                 `json:"schedule_kind"` // "at" | "every" | "cron"
	ScheduleConfig    map[string]interface{} `json:"schedule_config"`
	Payload           map[string]interface{} `json:"payload"`
	DeleteAfterRun    bool                   `json:"delete_after_run"`
	ConsecutiveErrors int                    `json:"consecutive_errors"`
	LastRunAt         time.Time              `json:"last_run_at"`
	NextRunAt         time.Time              `json:"next_run_at"`
}

// 定时任务服务
type CronService struct {
	cronFile    string
	jobs        []CronJob
	outputQueue []string
	queueLock   sync.Mutex
	runLog      string
	cron        *cron.Cron
	watcher     *fsnotify.Watcher // 文件系统监控器
}

// 工作区目录
var workspaceDir = "workspace"

// 初始化心跳运行器
func NewHeartbeatRunner(laneLock *sync.Mutex) *HeartbeatRunner {
	heartbeatPath := filepath.Join(workspaceDir, "HEARTBEAT.md")

	// 初始化fsnotify
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		fmt.Printf("Error creating watcher: %v\n", err)
	}

	// 监控工作目录
	if watcher != nil {
		if err := watcher.Add(workspaceDir); err != nil {
			fmt.Printf("Error watching workspace directory: %v\n", err)
		}
	}

	return &HeartbeatRunner{
		workspace:     workspaceDir,
		heartbeatPath: heartbeatPath,
		laneLock:      laneLock,
		interval:      30 * time.Minute, // 默认30分钟
		activeHours:   [2]int{9, 22},    // 默认9:00-22:00
		maxQueueSize:  10,
		lastRunAt:     time.Time{},
		running:       false,
		stopped:       false,
		thread:        make(chan struct{}),
		outputQueue:   make([]string, 0),
		watcher:       watcher,
	}
}

// 检查是否应该运行心跳
func (h *HeartbeatRunner) shouldRun() (bool, string) {
	// 检查HEARTBEAT.md是否存在
	if _, err := os.Stat(h.heartbeatPath); os.IsNotExist(err) {
		return false, "HEARTBEAT.md not found"
	}

	// 检查HEARTBEAT.md是否为空
	content, err := os.ReadFile(h.heartbeatPath)
	if err != nil || len(content) == 0 {
		return false, "HEARTBEAT.md is empty"
	}

	// 检查间隔
	if !h.lastRunAt.IsZero() {
		elapsed := time.Since(h.lastRunAt)
		if elapsed < h.interval {
			return false, fmt.Sprintf("interval not elapsed (%v remaining)", h.interval-elapsed)
		}
	}

	// 检查活跃时间
	hour := time.Now().Hour()
	s, e := h.activeHours[0], h.activeHours[1]
	inHours := (s <= hour && hour < e) || (s > e && (hour >= s || hour < e))
	if !inHours {
		return false, fmt.Sprintf("outside active hours (%d:00-%d:00)", s, e)
	}

	// 检查是否正在运行
	if h.running {
		return false, "already running"
	}

	return true, "all checks passed"
}

// 解析心跳响应
func (h *HeartbeatRunner) parseResponse(response string) string {
	// HEARTBEAT_OK 表示没有需要报告的内容
	if len(response) > 12 && response[:12] == "HEARTBEAT_OK" {
		stripped := response[12:]
		if len(stripped) > 5 {
			return stripped
		}
		return ""
	}
	return response
}

// 构建心跳提示
func (h *HeartbeatRunner) buildHeartbeatPrompt() (string, string) {
	instructions, err := os.ReadFile(h.heartbeatPath)
	if err != nil {
		return "", ""
	}

	// 加载记忆
	memPath := filepath.Join(h.workspace, "MEMORY.md")
	memContent := ""
	if mem, err := os.ReadFile(memPath); err == nil && len(mem) > 0 {
		memContent = fmt.Sprintf("## Known Context\n\n%s\n\n", string(mem))
	}

	// 构建额外信息
	_ = fmt.Sprintf("%sCurrent time: %s", memContent, time.Now().Format("2006-01-02 15:04:05"))
	sysPrompt := "You are a helpful assistant performing a background check."

	return string(instructions), sysPrompt
}

// 执行心跳
func (h *HeartbeatRunner) execute() {
	// 尝试获取锁，非阻塞
	if !h.laneLock.TryLock() {
		return
	}
	defer h.laneLock.Unlock()

	h.running = true
	defer func() {
		h.running = false
		h.lastRunAt = time.Now()
	}()

	instructions, _ := h.buildHeartbeatPrompt()
	if instructions == "" {
		return
	}

	// 这里应该调用模型，但为了简化，我们暂时返回一个模拟响应
	// 实际实现中应该调用 CallModel 函数
	response := "HEARTBEAT_OK"
	meaningful := h.parseResponse(response)

	if meaningful == "" {
		return
	}

	if meaningful == h.lastOutput {
		return
	}

	h.lastOutput = meaningful
	h.queueLock.Lock()
	h.outputQueue = append(h.outputQueue, meaningful)
	h.queueLock.Unlock()
}

// 启动心跳
func (h *HeartbeatRunner) Start() {
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if h.stopped {
					return
				}
				if ok, _ := h.shouldRun(); ok {
					h.execute()
				}
			case <-h.thread:
				return
			}
		}
	}()
}

// 停止心跳
func (h *HeartbeatRunner) Stop() {
	h.stopped = true
	close(h.thread)
	if h.watcher != nil {
		h.watcher.Close()
	}
}

// 清空输出队列
func (h *HeartbeatRunner) DrainOutput() []string {
	h.queueLock.Lock()
	defer h.queueLock.Unlock()

	items := make([]string, len(h.outputQueue))
	copy(items, h.outputQueue)
	h.outputQueue = make([]string, 0)
	return items
}

// 手动触发心跳
func (h *HeartbeatRunner) Trigger() string {
	if !h.laneLock.TryLock() {
		return "main lane occupied, cannot trigger"
	}
	defer h.laneLock.Unlock()

	h.running = true
	defer func() {
		h.running = false
		h.lastRunAt = time.Now()
	}()

	instructions, _ := h.buildHeartbeatPrompt()
	if instructions == "" {
		return "HEARTBEAT.md is empty"
	}

	// 这里应该调用模型，但为了简化，我们暂时返回一个模拟响应
	response := "HEARTBEAT_OK"
	meaningful := h.parseResponse(response)

	if meaningful == "" {
		return "HEARTBEAT_OK (nothing to report)"
	}

	if meaningful == h.lastOutput {
		return "duplicate content (skipped)"
	}

	h.lastOutput = meaningful
	h.queueLock.Lock()
	h.outputQueue = append(h.outputQueue, meaningful)
	h.queueLock.Unlock()

	return fmt.Sprintf("triggered, output queued (%d chars)", len(meaningful))
}

// 获取心跳状态
func (h *HeartbeatRunner) Status() map[string]interface{} {
	elapsed := time.Since(h.lastRunAt)
	nextIn := h.interval - elapsed
	if nextIn < 0 {
		nextIn = 0
	}

	ok, reason := h.shouldRun()
	h.queueLock.Lock()
	qsize := len(h.outputQueue)
	h.queueLock.Unlock()

	return map[string]interface{}{
		"enabled":      h.heartbeatPath != "",
		"running":      h.running,
		"should_run":   ok,
		"reason":       reason,
		"last_run":     h.lastRunAt.Format(time.RFC3339),
		"next_in":      nextIn.String(),
		"interval":     h.interval.String(),
		"active_hours": fmt.Sprintf("%d:00-%d:00", h.activeHours[0], h.activeHours[1]),
		"queue_size":   qsize,
	}
}

// 初始化定时任务服务
func NewCronService() *CronService {
	cronFile := filepath.Join(workspaceDir, "CRON.json")
	runLog := filepath.Join(workspaceDir, "cron", "cron-runs.jsonl")

	// 创建cron目录
	if err := os.MkdirAll(filepath.Dir(runLog), 0755); err != nil {
		fmt.Printf("Error creating cron directory: %v\n", err)
	}

	// 创建cron实例
	c := cron.New()

	// 初始化fsnotify
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		fmt.Printf("Error creating watcher: %v\n", err)
	}

	// 监控工作目录
	if watcher != nil {
		if err := watcher.Add(workspaceDir); err != nil {
			fmt.Printf("Error watching workspace directory: %v\n", err)
		}
	}

	cs := &CronService{
		cronFile:    cronFile,
		jobs:        make([]CronJob, 0),
		outputQueue: make([]string, 0),
		runLog:      runLog,
		cron:        c,
		watcher:     watcher,
	}

	cs.loadJobs()
	// 启动cron
	cs.cron.Start()

	// 启动文件监控协程
	go cs.startWatcher()

	return cs
}

// 加载定时任务
func (cs *CronService) loadJobs() {
	cs.jobs = make([]CronJob, 0)

	if _, err := os.Stat(cs.cronFile); os.IsNotExist(err) {
		return
	}

	content, err := os.ReadFile(cs.cronFile)
	if err != nil {
		fmt.Printf("Error reading CRON.json: %v\n", err)
		return
	}

	var data map[string]interface{}
	if err := json.Unmarshal(content, &data); err != nil {
		fmt.Printf("Error parsing CRON.json: %v\n", err)
		return
	}

	jobsData, ok := data["jobs"].([]interface{})
	if !ok {
		return
	}

	now := time.Now()
	for _, jd := range jobsData {
		jobMap, ok := jd.(map[string]interface{})
		if !ok {
			continue
		}

		sched, ok := jobMap["schedule"].(map[string]interface{})
		if !ok {
			continue
		}

		kind, ok := sched["kind"].(string)
		if !ok || (kind != "at" && kind != "every" && kind != "cron") {
			continue
		}

		job := CronJob{
			ID:             jobMap["id"].(string),
			Name:           jobMap["name"].(string),
			Enabled:        jobMap["enabled"].(bool),
			ScheduleKind:   kind,
			ScheduleConfig: sched,
			Payload:        jobMap["payload"].(map[string]interface{}),
		}

		if deleteAfterRun, ok := jobMap["delete_after_run"].(bool); ok {
			job.DeleteAfterRun = deleteAfterRun
		}

		job.NextRunAt = cs.computeNext(&job, now)
		cs.jobs = append(cs.jobs, job)

		// 添加到cron调度
		if job.Enabled && !job.NextRunAt.IsZero() && job.ScheduleKind == "cron" {
			cs.scheduleJob(&job)
		}
	}
}

// 计算下次运行时间
func (cs *CronService) computeNext(job *CronJob, now time.Time) time.Time {
	cfg := job.ScheduleConfig

	switch job.ScheduleKind {
	case "at":
		if atStr, ok := cfg["at"].(string); ok {
			if at, err := time.Parse(time.RFC3339, atStr); err == nil && at.After(now) {
				return at
			}
		}
	case "every":
		everySeconds := 3600.0 // 默认1小时
		if every, ok := cfg["every_seconds"].(float64); ok {
			everySeconds = every
		}

		var anchor time.Time
		if anchorStr, ok := cfg["anchor"].(string); ok {
			if a, err := time.Parse(time.RFC3339, anchorStr); err == nil {
				anchor = a
			} else {
				anchor = now
			}
		} else {
			anchor = now
		}

		if now.Before(anchor) {
			return anchor
		}

		steps := int((now.Sub(anchor).Seconds() / everySeconds)) + 1
		return anchor.Add(time.Duration(steps) * time.Duration(everySeconds) * time.Second)
	case "cron":
		if expr, ok := cfg["expr"].(string); ok && expr != "" {
			if _, err := cron.ParseStandard(expr); err == nil {
				// 对于cron表达式，我们暂时返回一个默认值
				// 实际实现中应该使用cron库来计算下次运行时间
				return now.Add(24 * time.Hour)
			}
		}
	}

	return time.Time{}
}

// 调度任务
func (cs *CronService) scheduleJob(job *CronJob) {
	if expr, ok := job.ScheduleConfig["expr"].(string); ok && expr != "" {
		cs.cron.AddFunc(expr, func() {
			cs.runJob(job)
		})
	}
}

// 运行任务
func (cs *CronService) runJob(job *CronJob) {
	now := time.Now()
	payload := job.Payload

	var output, status, errMsg string
	status = "ok"

	defer func() {
		job.LastRunAt = now
		if status == "error" {
			job.ConsecutiveErrors++
			if job.ConsecutiveErrors >= 5 {
				job.Enabled = false
				msg := fmt.Sprintf("Job '%s' auto-disabled after %d consecutive errors: %s", job.Name, job.ConsecutiveErrors, errMsg)
				fmt.Println(msg)
				cs.queueLock.Lock()
				cs.outputQueue = append(cs.outputQueue, msg)
				cs.queueLock.Unlock()
			}
		} else {
			job.ConsecutiveErrors = 0
		}

		job.NextRunAt = cs.computeNext(job, now)

		// 记录运行日志
		entry := map[string]interface{}{
			"job_id":         job.ID,
			"run_at":         now.Format(time.RFC3339),
			"status":         status,
			"output_preview": output[:min(200, len(output))],
		}
		if errMsg != "" {
			entry["error"] = errMsg
		}

		if data, err := json.Marshal(entry); err == nil {
			if f, err := os.OpenFile(cs.runLog, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
				defer f.Close()
				f.WriteString(string(data) + "\n")
			}
		}

		if output != "" && status != "skipped" {
			cs.queueLock.Lock()
			cs.outputQueue = append(cs.outputQueue, fmt.Sprintf("[%s] %s", job.Name, output))
			cs.queueLock.Unlock()
		}
	}()

	kind, ok := payload["kind"].(string)
	if !ok {
		output, status, errMsg = "[unknown kind]", "error", "unknown kind"
		return
	}

	switch kind {
	case "agent_turn":
		msg, ok := payload["message"].(string)
		if !ok || msg == "" {
			output, status = "[empty message]", "skipped"
			return
		}

		// 这里应该调用模型，但为了简化，我们暂时返回一个模拟响应
		_ = fmt.Sprintf("You are performing a scheduled background task. Be concise. Current time: %s", now.Format("2006-01-02 15:04:05"))
		output = "Cron job executed successfully"
	case "system_event":
		text, ok := payload["text"].(string)
		if !ok || text == "" {
			status = "skipped"
			return
		}
		output = text
	default:
		output, status, errMsg = fmt.Sprintf("[unknown kind: %s]", kind), "error", fmt.Sprintf("unknown kind: %s", kind)
	}
}

// 手动触发任务
func (cs *CronService) TriggerJob(jobID string) string {
	for i, job := range cs.jobs {
		if job.ID == jobID {
			cs.runJob(&cs.jobs[i])
			return fmt.Sprintf("'%s' triggered (errors=%d)", job.Name, cs.jobs[i].ConsecutiveErrors)
		}
	}
	return fmt.Sprintf("Job '%s' not found", jobID)
}

// 清空输出队列
func (cs *CronService) DrainOutput() []string {
	cs.queueLock.Lock()
	defer cs.queueLock.Unlock()

	items := make([]string, len(cs.outputQueue))
	copy(items, cs.outputQueue)
	cs.outputQueue = make([]string, 0)
	return items
}

// 列出任务
func (cs *CronService) ListJobs() []map[string]interface{} {
	now := time.Now()
	result := make([]map[string]interface{}, 0)

	for _, j := range cs.jobs {
		var nextIn *time.Duration
		if !j.NextRunAt.IsZero() {
			diff := j.NextRunAt.Sub(now)
			if diff > 0 {
				nextIn = &diff
			}
		}

		jobInfo := map[string]interface{}{
			"id":       j.ID,
			"name":     j.Name,
			"enabled":  j.Enabled,
			"kind":     j.ScheduleKind,
			"errors":   j.ConsecutiveErrors,
			"last_run": j.LastRunAt.Format(time.RFC3339),
			"next_run": j.NextRunAt.Format(time.RFC3339),
		}

		if nextIn != nil {
			jobInfo["next_in"] = nextIn.String()
		}

		result = append(result, jobInfo)
	}

	return result
}

// 启动文件监控协程
func (cs *CronService) startWatcher() {
	if cs.watcher == nil {
		return
	}

	for {
		select {
		case event, ok := <-cs.watcher.Events:
			if !ok {
				return
			}

			// 只关心写入和创建事件
			if event.Op&(fsnotify.Write|fsnotify.Create) != 0 {
				// 检查是否是CRON.json文件
				if filepath.Base(event.Name) == "CRON.json" {
					fmt.Println("[cron] CRON.json changed, reloading jobs...")
					cs.reloadJobs()
					fmt.Println("[cron] Jobs reloaded successfully")
				}
			}

		case err, ok := <-cs.watcher.Errors:
			if !ok {
				return
			}
			fmt.Printf("[cron] Watcher error: %v\n", err)
		}
	}
}

// 重新加载定时任务
func (cs *CronService) reloadJobs() {
	// 停止当前的cron服务
	cs.cron.Stop()

	// 创建新的cron实例
	cs.cron = cron.New()

	// 重新加载任务
	cs.loadJobs()

	// 启动新的cron服务
	cs.cron.Start()
}

// 停止定时任务服务
func (cs *CronService) Stop() {
	cs.cron.Stop()
	if cs.watcher != nil {
		cs.watcher.Close()
	}
}

// 辅助函数：获取最小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
