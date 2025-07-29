package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
	"gopkg.in/telebot.v3"
)

// Config 配置结构体
type Config struct {
	BotToken      string `json:"bot_token"`
	ChatID        int64  `json:"chat_id"`
	AdminID       int64  `json:"admin_id"`
	ReportTime    string `json:"report_time"`
	CustomMessage string `json:"custom_message"`
	CPUThreshold  int    `json:"cpu_threshold"`
	MemThreshold  int    `json:"mem_threshold"`
	AlertInterval int    `json:"alert_interval"`
}

// UserState 用户交互状态
type UserState struct {
	WaitingFor string
}

// SystemInfo 系统信息缓存
type SystemInfo struct {
	CPUPercent   float64
	MemInfo      *mem.VirtualMemoryStat
	DiskInfo     *disk.UsageStat
	NetInfo      *NetworkInfo
	HostInfo     *host.InfoStat
	LocationInfo *LocationInfo
	UpdateTime   time.Time
}

// ServerMonitor 服务器监控器
type ServerMonitor struct {
	bot           *telebot.Bot
	config        *Config
	lastStats     *NetStats
	lastAlertTime map[string]time.Time
	userState     map[int64]*UserState
	systemInfo    *SystemInfo
	beijingTZ     *time.Location
}

// NetStats 网络统计
type NetStats struct {
	BytesSent uint64
	BytesRecv uint64
	Timestamp time.Time
}

// LocationInfo IP 地理位置信息
type LocationInfo struct {
	IP       string
	Location string
	Country  string
}

func main() {
	config, err := loadConfig()
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	monitor, err := NewServerMonitor(config)
	if err != nil {
		log.Fatalf("创建监控器失败: %v", err)
	}

	monitor.Start()
}

// loadConfig 加载配置
func loadConfig() (*Config, error) {
	config := &Config{
		ReportTime:    "15:00",
		CPUThreshold:  80,
		MemThreshold:  80,
		AlertInterval: 10,
		CustomMessage: "🖥️ 服务器状态报告",
	}

	if token := os.Getenv("BOT_TOKEN"); token != "" {
		config.BotToken = token
	}
	if chatID := os.Getenv("CHAT_ID"); chatID != "" {
		if id, err := strconv.ParseInt(chatID, 10, 64); err == nil {
			config.ChatID = id
		}
	}
	if adminID := os.Getenv("ADMIN_ID"); adminID != "" {
		if id, err := strconv.ParseInt(adminID, 10, 64); err == nil {
			config.AdminID = id
		}
	}

	if data, err := os.ReadFile("config.json"); err == nil {
		if err := json.Unmarshal(data, config); err != nil {
			log.Printf("解析 config.json 失败: %v", err)
		}
	}

	if data, err := os.ReadFile("user_config.json"); err == nil {
		var userConfig map[string]interface{}
		if err := json.Unmarshal(data, &userConfig); err == nil {
			if reportTime, ok := userConfig["report_time"].(string); ok {
				config.ReportTime = reportTime
			}
			if customMessage, ok := userConfig["custom_message"].(string); ok {
				config.CustomMessage = customMessage
			}
			if cpuThreshold, ok := userConfig["cpu_threshold"].(float64); ok {
				config.CPUThreshold = int(cpuThreshold)
			}
			if memThreshold, ok := userConfig["mem_threshold"].(float64); ok {
				config.MemThreshold = int(memThreshold)
			}
			if alertInterval, ok := userConfig["alert_interval"].(float64); ok && alertInterval > 0 {
				config.AlertInterval = int(alertInterval)
			}
			log.Printf("已加载用户配置文件")
		}
	}

	// 验证必要配置
	if config.BotToken == "" {
		return nil, fmt.Errorf("必须设置 BOT_TOKEN")
	}
	if config.ChatID == 0 {
		return nil, fmt.Errorf("必须设置 CHAT_ID")
	}

	// 如果没有设置AdminID，默认使用ChatID
	if config.AdminID == 0 {
		config.AdminID = config.ChatID
	}

	return config, nil
}

// NewServerMonitor 创建新的服务器监控器
func NewServerMonitor(config *Config) (*ServerMonitor, error) {
	pref := telebot.Settings{
		Token:  config.BotToken,
		Poller: &telebot.LongPoller{Timeout: 10 * time.Second},
	}

	bot, err := telebot.NewBot(pref)
	if err != nil {
		return nil, err
	}

	bot.SetCommands([]telebot.Command{
		{Text: "start", Description: "开始使用"},
	})

	monitor := &ServerMonitor{
		bot:           bot,
		config:        config,
		lastAlertTime: make(map[string]time.Time),
		userState:     make(map[int64]*UserState),
		systemInfo:    &SystemInfo{},
		beijingTZ:     time.FixedZone("CST", 8*3600),
	}

	monitor.initNetStats()

	return monitor, nil
}

// isAdmin 检查用户是否为管理员
func (m *ServerMonitor) isAdmin(userID int64) bool {
	return userID == m.config.AdminID
}

// Start 启动监控器
func (m *ServerMonitor) Start() {
	mainMenu := &telebot.ReplyMarkup{}
	btnStatus := mainMenu.Data("📊 实时状态", "status")
	btnConfig := mainMenu.Data("⚙️ 配置设置", "config")
	mainMenu.Inline(
		mainMenu.Row(btnStatus),
		mainMenu.Row(btnConfig),
	)

	configMenu := &telebot.ReplyMarkup{}
	btnReportTime := configMenu.Data("⏰ 报告时间", "set_report_time")
	btnCustomMsg := configMenu.Data("💬 自定义消息", "set_custom_message")
	btnCPUThreshold := configMenu.Data("🔥 CPU阈值", "set_cpu_threshold")
	btnMemThreshold := configMenu.Data("💾 内存阈值", "set_mem_threshold")
	btnAlertInterval := configMenu.Data("⏱️ 告警间隔", "set_alert_interval")
	btnBack := configMenu.Data("🔙 返回主菜单", "back_main")
	configMenu.Inline(
		configMenu.Row(btnReportTime, btnCustomMsg),
		configMenu.Row(btnCPUThreshold, btnMemThreshold),
		configMenu.Row(btnAlertInterval),
		configMenu.Row(btnBack),
	)

	m.bot.Handle("/start", m.createAsyncHandler(func(c telebot.Context) error {
		msg := m.getMainMenuMessage()
		return c.Send(msg, mainMenu)
	}))

	m.bot.Handle(&btnStatus, m.createAsyncHandler(func(c telebot.Context) error {
		m.updateSystemInfo()
		report := m.generateReport()
		return c.Edit(report, &telebot.SendOptions{ParseMode: telebot.ModeMarkdown}, mainMenu)
	}))

	m.bot.Handle(&btnConfig, func(c telebot.Context) error {
		if !m.isAdmin(c.Sender().ID) {
			return c.Send("❌ 权限不足，只有管理员可以执行此操作")
		}
		msg := m.getConfigMenuMessage()
		return c.Edit(msg, &telebot.SendOptions{ParseMode: telebot.ModeMarkdown}, configMenu)
	})

	m.bot.Handle(&btnBack, m.createAsyncHandler(func(c telebot.Context) error {
		msg := m.getMainMenuMessage()
		return c.Edit(msg, &telebot.SendOptions{ParseMode: telebot.ModeMarkdown}, mainMenu)
	}))

	configHandlers := map[*telebot.Btn]struct {
		configType string
		prompt     string
	}{
		&btnReportTime:    {"report_time", "⏰ 请输入新的报告时间（格式：HH:MM，如 15:00）:"},
		&btnCustomMsg:     {"custom_message", "💬 请输入自定义消息内容:"},
		&btnCPUThreshold:  {"cpu_threshold", "🔥 请输入CPU告警阈值（1-100的整数）:"},
		&btnMemThreshold:  {"mem_threshold", "💾 请输入内存告警阈值（1-100的整数）:"},
		&btnAlertInterval: {"alert_interval", "⏱️ 请输入告警间隔时间（分钟，建议5-60分钟）:"},
	}

	for btn, config := range configHandlers {
		configType := config.configType
		prompt := config.prompt
		m.bot.Handle(btn, func(c telebot.Context) error {
			if !m.isAdmin(c.Sender().ID) {
				return c.Send("❌ 权限不足，只有管理员可以执行此操作")
			}
			m.userState[c.Chat().ID] = &UserState{WaitingFor: configType}
			return c.Edit(prompt, &telebot.SendOptions{ParseMode: telebot.ModeMarkdown})
		})
	}

	m.bot.Handle(telebot.OnText, func(c telebot.Context) error {
		chatID := c.Chat().ID
		state, exists := m.userState[chatID]
		if !exists || state.WaitingFor == "" {
			return nil
		}

		if !m.isAdmin(c.Sender().ID) {
			delete(m.userState, chatID)
			return c.Send("❌ 权限不足，只有管理员可以修改配置")
		}

		return m.handleConfigInput(c, mainMenu, configMenu)
	})

	go m.startScheduledReport()
	go m.startRealTimeAlert()
	go m.startSystemInfoUpdater()

	log.Printf("机器人启动成功，定时报告时间: %s", m.config.ReportTime)
	m.bot.Start()
}

// initNetStats 初始化网络统计
func (m *ServerMonitor) initNetStats() {
	stats, err := net.IOCounters(false)
	if err != nil || len(stats) == 0 {
		return
	}

	m.lastStats = &NetStats{
		BytesSent: stats[0].BytesSent,
		BytesRecv: stats[0].BytesRecv,
		Timestamp: time.Now(),
	}
}

// updateSystemInfo 更新系统信息缓存
func (m *ServerMonitor) updateSystemInfo() {
	now := time.Now()
	if now.Sub(m.systemInfo.UpdateTime) < 10*time.Second {
		return
	}

	type result struct {
		cpu      float64
		mem      *mem.VirtualMemoryStat
		disk     *disk.UsageStat
		net      *NetworkInfo
		host     *host.InfoStat
		location *LocationInfo
	}

	resultChan := make(chan result, 1)

	go func() {
		var r result

		cpuChan := make(chan float64, 1)
		memChan := make(chan *mem.VirtualMemoryStat, 1)
		diskChan := make(chan *disk.UsageStat, 1)
		netChan := make(chan *NetworkInfo, 1)
		hostChan := make(chan *host.InfoStat, 1)
		locationChan := make(chan *LocationInfo, 1)

		go func() { cpuChan <- m.getCPUUsage() }()
		go func() { memChan <- m.getMemoryInfo() }()
		go func() { diskChan <- m.getDiskInfo() }()
		go func() { netChan <- m.getNetworkInfo() }()
		go func() { hostChan <- m.getHostInfo() }()
		go func() { locationChan <- m.getLocationInfo() }()

		r.cpu = <-cpuChan
		r.mem = <-memChan
		r.disk = <-diskChan
		r.net = <-netChan
		r.host = <-hostChan
		r.location = <-locationChan

		resultChan <- r
	}()

	select {
	case r := <-resultChan:
		m.systemInfo = &SystemInfo{
			CPUPercent:   r.cpu,
			MemInfo:      r.mem,
			DiskInfo:     r.disk,
			NetInfo:      r.net,
			HostInfo:     r.host,
			LocationInfo: r.location,
			UpdateTime:   now,
		}
	case <-time.After(10 * time.Second):
		log.Printf("获取系统信息超时")
	}
}

func (m *ServerMonitor) formatBeijingTime(t time.Time) string {
	return t.In(m.beijingTZ).Format("2006-01-02 15:04:05")
}

// createAsyncHandler 创建异步处理器
func (m *ServerMonitor) createAsyncHandler(handler func(telebot.Context) error) func(telebot.Context) error {
	return func(c telebot.Context) error {
		go func() {
			if err := handler(c); err != nil {
				log.Printf("异步处理错误: %v", err)
			}
		}()
		return nil
	}
}

// startSystemInfoUpdater 启动系统信息定期更新器
func (m *ServerMonitor) startSystemInfoUpdater() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		m.updateSystemInfo()
	}
}

// startRealTimeAlert 启动实时告警监控
func (m *ServerMonitor) startRealTimeAlert() {
	ticker := time.NewTicker(3 * time.Second) // 每3秒检查一次告警，保持高实时性
	defer ticker.Stop()

	for range ticker.C {
		m.checkAndSendRealTimeAlert()
	}
}

// checkAndSendRealTimeAlert 检查并发送实时告警
func (m *ServerMonitor) checkAndSendRealTimeAlert() {
	now := time.Now()
	alertInterval := time.Duration(m.config.AlertInterval) * time.Minute
	cpuPercent := m.getCPUUsage()
	memInfo := m.getMemoryInfo()

	if cpuPercent > float64(m.config.CPUThreshold) {
		if m.shouldSendAlert("cpu", now, alertInterval) {
			alertMsg := fmt.Sprintf("🚨 *CPU告警*\n\n"+
				"当前使用率: %.1f%%\n"+
				"告警阈值: %d%%\n"+
				"时间: %s",
				cpuPercent,
				m.config.CPUThreshold,
				m.formatBeijingTime(now))
			go m.sendMessage(alertMsg)
			m.lastAlertTime["cpu"] = now
			log.Printf("发送CPU告警: %.1f%%", cpuPercent)
		}
	}

	if memInfo.UsedPercent > float64(m.config.MemThreshold) {
		if m.shouldSendAlert("mem", now, alertInterval) {
			alertMsg := fmt.Sprintf("🚨 *内存告警*\n\n"+
				"当前使用率: %.1f%%\n"+
				"已用内存: %.1fMB/%.1fMB\n"+
				"告警阈值: %d%%\n"+
				"时间: %s",
				memInfo.UsedPercent,
				float64(memInfo.Used)/1024/1024,
				float64(memInfo.Total)/1024/1024,
				m.config.MemThreshold,
				m.formatBeijingTime(now))
			go m.sendMessage(alertMsg)
			m.lastAlertTime["mem"] = now
			log.Printf("发送内存告警: %.1f%%", memInfo.UsedPercent)
		}
	}
}

// shouldSendAlert 检查是否应该发送告警
func (m *ServerMonitor) shouldSendAlert(alertType string, now time.Time, interval time.Duration) bool {
	lastAlert, exists := m.lastAlertTime[alertType]
	return !exists || now.Sub(lastAlert) >= interval
}

// startScheduledReport 启动定时报告
func (m *ServerMonitor) startScheduledReport() {
	for {
		now := time.Now()
		beijingTime := now.In(m.beijingTZ)
		reportTime, err := time.Parse("15:04", m.config.ReportTime)
		if err != nil {
			log.Printf("解析报告时间失败: %v", err)
			time.Sleep(time.Minute)
			continue
		}

		targetTime := time.Date(
			beijingTime.Year(), beijingTime.Month(), beijingTime.Day(),
			reportTime.Hour(), reportTime.Minute(), 0, 0,
			m.beijingTZ,
		)

		if beijingTime.After(targetTime) {
			targetTime = targetTime.Add(24 * time.Hour)
		}

		waitDuration := targetTime.Sub(beijingTime)
		log.Printf("下次报告时间: %s (等待 %v)", targetTime.Format("2006-01-02 15:04:05"), waitDuration)

		time.Sleep(waitDuration)

		go func() {
			m.updateSystemInfo()
			report := m.generateReport()
			m.sendScheduledReport(report)
		}()
	}
}

// generateReport 生成监控报告
func (m *ServerMonitor) generateReport() string {
	var buf bytes.Buffer

	info := m.systemInfo

	buf.WriteString(fmt.Sprintf("🌍 *服务器位置*: %s (%s)\n", info.LocationInfo.Location, maskIP(info.LocationInfo.IP)))
	buf.WriteString(fmt.Sprintf("🕐 *更新时间*: %s\n\n", m.formatBeijingTime(time.Now())))

	cpuIcon := m.getStatusIcon(info.CPUPercent, float64(m.config.CPUThreshold))
	buf.WriteString(fmt.Sprintf("%s *CPU 使用率*: %.1f%%\n", cpuIcon, info.CPUPercent))

	memIcon := m.getStatusIcon(info.MemInfo.UsedPercent, float64(m.config.MemThreshold))
	buf.WriteString(fmt.Sprintf("%s *内存使用*: %.1fMB/%.1fMB (%.1f%%)\n",
		memIcon,
		float64(info.MemInfo.Used)/1024/1024,
		float64(info.MemInfo.Total)/1024/1024,
		info.MemInfo.UsedPercent))

	diskIcon := m.getStatusIcon(info.DiskInfo.UsedPercent, 80)
	buf.WriteString(fmt.Sprintf("%s *磁盘使用*: %.1fGB/%.1fGB (%.1f%%)\n",
		diskIcon,
		float64(info.DiskInfo.Used)/1024/1024/1024,
		float64(info.DiskInfo.Total)/1024/1024/1024,
		info.DiskInfo.UsedPercent))

	buf.WriteString(fmt.Sprintf("📊 *网络流量*: ↓%.2fGB ↑%.2fGB\n", info.NetInfo.RecvGB, info.NetInfo.SentGB))

	buf.WriteString(fmt.Sprintf("\n🖥️ *系统信息*:\n"))
	buf.WriteString(fmt.Sprintf("• 系统: %s\n", info.HostInfo.Platform))
	buf.WriteString(fmt.Sprintf("• 运行时间: %s\n", m.formatUptime(info.HostInfo.Uptime)))

	buf.WriteString("\n")
	buf.WriteString(m.config.CustomMessage)
	buf.WriteString("\n")

	return buf.String()
}

// getStatusIcon 根据使用率获取状态图标
func (m *ServerMonitor) getStatusIcon(current, threshold float64) string {
	if current > threshold {
		return "🔴"
	}
	return "💚"
}

// getCPUUsage 获取 CPU 使用率
func (m *ServerMonitor) getCPUUsage() float64 {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	percent, err := cpu.PercentWithContext(ctx, time.Second, false)
	if err != nil || len(percent) == 0 {
		return 0
	}
	return percent[0]
}

// getMemoryInfo 获取内存信息
func (m *ServerMonitor) getMemoryInfo() *mem.VirtualMemoryStat {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	memInfo, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return &mem.VirtualMemoryStat{}
	}
	return memInfo
}

// getDiskInfo 获取磁盘信息
func (m *ServerMonitor) getDiskInfo() *disk.UsageStat {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	diskInfo, err := disk.UsageWithContext(ctx, "/")
	if err != nil {
		return &disk.UsageStat{}
	}
	return diskInfo
}

// NetworkInfo 网络信息
type NetworkInfo struct {
	SentGB float64
	RecvGB float64
}

// getNetworkInfo 获取网络信息
func (m *ServerMonitor) getNetworkInfo() *NetworkInfo {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	stats, err := net.IOCountersWithContext(ctx, false)
	if err != nil || len(stats) == 0 {
		return &NetworkInfo{}
	}

	return &NetworkInfo{
		SentGB: float64(stats[0].BytesSent) / 1024 / 1024 / 1024,
		RecvGB: float64(stats[0].BytesRecv) / 1024 / 1024 / 1024,
	}
}

// getHostInfo 获取主机信息
func (m *ServerMonitor) getHostInfo() *host.InfoStat {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	hostInfo, err := host.InfoWithContext(ctx)
	if err != nil {
		return &host.InfoStat{}
	}
	return hostInfo
}

// getLocationInfo 获取位置信息
func (m *ServerMonitor) getLocationInfo() *LocationInfo {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("https://www.cloudflare.com/cdn-cgi/trace")
	if err != nil {
		return &LocationInfo{IP: "未知", Location: "未知"}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return &LocationInfo{IP: "未知", Location: "未知"}
	}

	lines := strings.Split(string(body), "\n")
	info := &LocationInfo{}

	for _, line := range lines {
		if strings.HasPrefix(line, "ip=") {
			info.IP = strings.TrimPrefix(line, "ip=")
		} else if strings.HasPrefix(line, "loc=") {
			info.Location = strings.TrimPrefix(line, "loc=")
		} else if strings.HasPrefix(line, "colo=") {
			info.Country = strings.TrimPrefix(line, "colo=")
		}
	}

	if info.IP == "" {
		info.IP = "未知"
	}
	if info.Location == "" {
		info.Location = "未知"
	}

	return info
}

// IP地址脱敏
func maskIP(ip string) string {
	if strings.Count(ip, ".") == 3 {
		parts := strings.Split(ip, ".")
		return "x.x.x." + parts[3]
	}
	if strings.Contains(ip, ":") {
		ipStripped := strings.ReplaceAll(ip, ":", "")
		if len(ipStripped) > 8 {
			return "..." + ipStripped[len(ipStripped)-8:]
		}
		return "..." + ipStripped
	}

	return ip
}

// formatUptime 格式化运行时间
func (m *ServerMonitor) formatUptime(uptime uint64) string {
	duration := time.Duration(uptime) * time.Second
	days := int(duration.Hours()) / 24
	hours := int(duration.Hours()) % 24
	minutes := int(duration.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%d天%d小时%d分钟", days, hours, minutes)
	} else if hours > 0 {
		return fmt.Sprintf("%d小时%d分钟", hours, minutes)
	} else {
		return fmt.Sprintf("%d分钟", minutes)
	}
}

// sendMessage 发送消息
func (m *ServerMonitor) sendMessage(message string) {
	_, err := m.bot.Send(&telebot.Chat{ID: m.config.ChatID}, message, &telebot.SendOptions{
		ParseMode:             telebot.ModeMarkdown,
		DisableWebPagePreview: true, // 关闭链接预览
	})
	if err != nil {
		log.Printf("发送消息失败: %v", err)
	}
}

// sendScheduledReport 发送定时报告
func (m *ServerMonitor) sendScheduledReport(message string) {
	_, err := m.bot.Send(&telebot.Chat{ID: m.config.ChatID}, message, &telebot.SendOptions{
		ParseMode:             telebot.ModeMarkdown,
		DisableWebPagePreview: true, // 关闭链接预览
	})
	if err != nil {
		log.Printf("发送定时报告失败: %v", err)
	} else {
		log.Printf("定时报告发送成功")
	}
}

// getMainMenuMessage 获取主菜单消息
func (m *ServerMonitor) getMainMenuMessage() string {
	return fmt.Sprintf("🤖 *服务器监控机器人*\n\n"+
		"📅 *定时报告时间*: %s (北京时间)\n"+
		"💬 *自定义消息*: %s\n"+
		"⚠️ *CPU 告警阈值*: %d%%\n"+
		"⚠️ *内存告警阈值*: %d%%\n"+
		"⏱️ *告警间隔*: %d分钟\n\n"+
		"请选择操作:",
		m.config.ReportTime,
		m.config.CustomMessage,
		m.config.CPUThreshold,
		m.config.MemThreshold,
		m.config.AlertInterval)
}

// getConfigMenuMessage 获取配置菜单消息
func (m *ServerMonitor) getConfigMenuMessage() string {
	return fmt.Sprintf("⚙️ *配置设置*\n\n"+
		"当前配置:\n"+
		"⏰ *报告时间*: %s\n"+
		"💬 *自定义消息*: %s\n"+
		"🔥 *CPU阈值*: %d%%\n"+
		"💾 *内存阈值*: %d%%\n"+
		"⏱️ *告警间隔*: %d分钟\n\n"+
		"点击下方按钮修改配置:",
		m.config.ReportTime,
		m.config.CustomMessage,
		m.config.CPUThreshold,
		m.config.MemThreshold,
		m.config.AlertInterval)
}

// handleConfigInput 处理配置输入
func (m *ServerMonitor) handleConfigInput(c telebot.Context, mainMenu, configMenu *telebot.ReplyMarkup) error {
	chatID := c.Chat().ID
	state, exists := m.userState[chatID]
	if !exists || state.WaitingFor == "" {
		return nil
	}

	input := strings.TrimSpace(c.Text())
	var success bool
	var errorMsg string

	switch state.WaitingFor {
	case "report_time":
		if m.validateTimeFormat(input) {
			m.config.ReportTime = input
			success = true
		} else {
			errorMsg = "❌ 时间格式错误，请使用 HH:MM 格式（如 15:00）"
		}

	case "custom_message":
		if len(input) > 0 && len(input) <= 100 {
			m.config.CustomMessage = input
			success = true
		} else {
			errorMsg = "❌ 消息长度应在1-100字符之间"
		}

	case "cpu_threshold":
		if threshold, err := strconv.Atoi(input); err == nil && threshold >= 1 && threshold <= 100 {
			m.config.CPUThreshold = threshold
			success = true
		} else {
			errorMsg = "❌ 请输入1-100之间的整数"
		}

	case "mem_threshold":
		if threshold, err := strconv.Atoi(input); err == nil && threshold >= 1 && threshold <= 100 {
			m.config.MemThreshold = threshold
			success = true
		} else {
			errorMsg = "❌ 请输入1-100之间的整数"
		}

	case "alert_interval":
		if interval, err := strconv.Atoi(input); err == nil && interval >= 1 && interval <= 1440 {
			m.config.AlertInterval = interval
			success = true
		} else {
			errorMsg = "❌ 请输入1-1440之间的整数（分钟）"
		}
	}

	delete(m.userState, chatID)

	if success {
		m.saveConfig()

		msg := fmt.Sprintf("✅ 配置已更新！\n\n%s", m.getConfigMenuMessage())
		return c.Send(msg, &telebot.SendOptions{ParseMode: telebot.ModeMarkdown}, configMenu)
	} else {
		msg := fmt.Sprintf("%s\n\n%s", errorMsg, m.getConfigMenuMessage())
		return c.Send(msg, &telebot.SendOptions{ParseMode: telebot.ModeMarkdown}, configMenu)
	}
}

func (m *ServerMonitor) validateTimeFormat(timeStr string) bool {
	_, err := time.Parse("15:04", timeStr)
	return err == nil
}

func (m *ServerMonitor) saveConfig() {
	configData := map[string]interface{}{
		"report_time":    m.config.ReportTime,
		"custom_message": m.config.CustomMessage,
		"cpu_threshold":  m.config.CPUThreshold,
		"mem_threshold":  m.config.MemThreshold,
		"alert_interval": m.config.AlertInterval,
	}

	data, err := json.MarshalIndent(configData, "", "  ")
	if err != nil {
		log.Printf("序列化配置失败: %v", err)
		return
	}

	err = os.WriteFile("user_config.json", data, 0644)
	if err != nil {
		log.Printf("保存配置失败: %v", err)
	} else {
		log.Printf("配置已保存到 user_config.json")
	}
}
