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
	ReportTime    string `json:"report_time"`    // 格式: 15:00
	CustomMessage string `json:"custom_message"` // 自定义提示信息
	CPUThreshold  int    `json:"cpu_threshold"`  // CPU 阈值，默认 80
	MemThreshold  int    `json:"mem_threshold"`  // 内存阈值，默认 80
}

// ServerMonitor 服务器监控器
type ServerMonitor struct {
	bot       *telebot.Bot
	config    *Config
	lastStats *NetStats
	alertSent map[string]bool // 跟踪已发送的告警
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
	// 加载配置
	config, err := loadConfig()
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 创建监控器
	monitor, err := NewServerMonitor(config)
	if err != nil {
		log.Fatalf("创建监控器失败: %v", err)
	}

	// 启动监控器
	monitor.Start()
}

// loadConfig 加载配置
func loadConfig() (*Config, error) {
	config := &Config{
		ReportTime:    "15:00", // 默认下午3点
		CPUThreshold:  80,
		MemThreshold:  80,
		CustomMessage: "🖥️ 服务器状态报告",
	}

	// 从环境变量读取
	if token := os.Getenv("BOT_TOKEN"); token != "" {
		config.BotToken = token
	}
	if chatID := os.Getenv("CHAT_ID"); chatID != "" {
		if id, err := strconv.ParseInt(chatID, 10, 64); err == nil {
			config.ChatID = id
		}
	}
	if reportTime := os.Getenv("REPORT_TIME"); reportTime != "" {
		config.ReportTime = reportTime
	}
	if customMsg := os.Getenv("CUSTOM_MESSAGE"); customMsg != "" {
		config.CustomMessage = customMsg
	}
	if cpuThreshold := os.Getenv("CPU_THRESHOLD"); cpuThreshold != "" {
		if threshold, err := strconv.Atoi(cpuThreshold); err == nil {
			config.CPUThreshold = threshold
		}
	}
	if memThreshold := os.Getenv("MEM_THRESHOLD"); memThreshold != "" {
		if threshold, err := strconv.Atoi(memThreshold); err == nil {
			config.MemThreshold = threshold
		}
	}

	// 尝试从 config.json 读取
	if data, err := os.ReadFile("config.json"); err == nil {
		if err := json.Unmarshal(data, config); err != nil {
			log.Printf("解析 config.json 失败: %v", err)
		}
	}

	// 验证必要配置
	if config.BotToken == "" {
		return nil, fmt.Errorf("必须设置 BOT_TOKEN")
	}
	if config.ChatID == 0 {
		return nil, fmt.Errorf("必须设置 CHAT_ID")
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

	monitor := &ServerMonitor{
		bot:       bot,
		config:    config,
		alertSent: make(map[string]bool), // 初始化告警状态
	}

	// 初始化网络统计
	monitor.initNetStats()

	return monitor, nil
}

// Start 启动监控器
func (m *ServerMonitor) Start() {
	// 设置按钮
	menu := &telebot.ReplyMarkup{}
	btnStatus := menu.Data("📊 实时状态", "status")
	menu.Inline(menu.Row(btnStatus))

	// 处理 /start 命令
	m.bot.Handle("/start", func(c telebot.Context) error {
		msg := fmt.Sprintf("🤖 服务器监控机器人已启动！\n\n"+
			"📅 定时报告时间: %s (北京时间)\n"+
			"⚠️ CPU 告警阈值: %d%%\n"+
			"⚠️ 内存告警阈值: %d%%\n\n"+
			"点击下方按钮获取实时状态:",
			m.config.ReportTime,
			m.config.CPUThreshold,
			m.config.MemThreshold)
		return c.Send(msg, menu)
	})

	// 处理按钮点击
	m.bot.Handle(&btnStatus, func(c telebot.Context) error {
		report := m.generateReport()
		return c.Edit(report, &telebot.SendOptions{ParseMode: telebot.ModeMarkdown}, menu)
	})

	// 启动定时任务
	go m.startScheduledReport()
	go m.startRealTimeAlert() // 启动实时告警监控

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

// startRealTimeAlert 启动实时告警监控
func (m *ServerMonitor) startRealTimeAlert() {
	ticker := time.NewTicker(2 * time.Second) // 每2秒检查一次
	defer ticker.Stop()

	for range ticker.C {
		cpuPercent := m.getCPUUsage()
		memInfo := m.getMemoryInfo()

		// CPU告警检查
		if cpuPercent > float64(m.config.CPUThreshold) && !m.alertSent["cpu"] {
			m.sendMessage(fmt.Sprintf("🚨 *CPU告警*: 使用率达到 %.1f%%", cpuPercent))
			m.alertSent["cpu"] = true
		} else if cpuPercent <= float64(m.config.CPUThreshold-10) { // 降低10%后重置通知
			m.alertSent["cpu"] = false
		}

		// 内存告警检查
		if memInfo.UsedPercent > float64(m.config.MemThreshold) && !m.alertSent["mem"] {
			m.sendMessage(fmt.Sprintf("🚨 *内存告警*: 使用率达到 %.1f%%", memInfo.UsedPercent))
			m.alertSent["mem"] = true
		} else if memInfo.UsedPercent <= float64(m.config.MemThreshold-10) {
			m.alertSent["mem"] = false
		}
	}
}

// startScheduledReport 启动定时报告
func (m *ServerMonitor) startScheduledReport() {
	for {
		now := time.Now()
		// 转换为北京时间
		beijingTime := now.In(time.FixedZone("CST", 8*3600))
		
		// 解析报告时间
		reportTime, err := time.Parse("15:04", m.config.ReportTime)
		if err != nil {
			log.Printf("解析报告时间失败: %v", err)
			time.Sleep(time.Minute)
			continue
		}

		// 设置今天的报告时间
		targetTime := time.Date(
			beijingTime.Year(), beijingTime.Month(), beijingTime.Day(),
			reportTime.Hour(), reportTime.Minute(), 0, 0,
			time.FixedZone("CST", 8*3600),
		)

		// 如果已经过了今天的报告时间，设置为明天的报告时间
		if beijingTime.After(targetTime) {
			targetTime = targetTime.Add(24 * time.Hour)
		}

		// 计算等待时间
		waitDuration := targetTime.Sub(beijingTime)
		log.Printf("下次报告时间: %s (等待 %v)", targetTime.Format("2006-01-02 15:04:05"), waitDuration)

		time.Sleep(waitDuration)

		// 发送报告
		report := m.generateReport()
		m.sendMessage(report)
		
		// 检查是否需要发送告警
		m.checkAndSendAlert()
	}
}

// generateReport 生成监控报告
func (m *ServerMonitor) generateReport() string {
	var buf bytes.Buffer

	// 获取位置信息
	location := m.getLocationInfo()
	
	buf.WriteString(fmt.Sprintf("🌍 *服务器位置*: %s (%s)\n", location.Location, maskIP(location.IP)))
	buf.WriteString(fmt.Sprintf("🕐 *更新时间*: %s\n\n", time.Now().In(time.FixedZone("CST", 8*3600)).Format("2006-01-02 15:04:05")))

	// CPU 信息
	cpuPercent := m.getCPUUsage()
	cpuIcon := "💚"
	if cpuPercent > float64(m.config.CPUThreshold) {
		cpuIcon = "🔴"
	}
	buf.WriteString(fmt.Sprintf("%s *CPU 使用率*: %.1f%%\n", cpuIcon, cpuPercent))

	// 内存信息
	memInfo := m.getMemoryInfo()
	memIcon := "💚"
	if memInfo.UsedPercent > float64(m.config.MemThreshold) {
		memIcon = "🔴"
	}
	buf.WriteString(fmt.Sprintf("%s *内存使用*: %.1fMB/%.1fMB (%.1f%%)\n", 
		memIcon, 
		float64(memInfo.Used)/1024/1024,
		float64(memInfo.Total)/1024/1024,
		memInfo.UsedPercent))

	// 磁盘信息
	diskInfo := m.getDiskInfo()
	diskIcon := "💚"
	if diskInfo.UsedPercent > 80 {
		diskIcon = "🔴"
	}
	buf.WriteString(fmt.Sprintf("%s *磁盘使用*: %.1fGB/%.1fGB (%.1f%%)\n", 
		diskIcon,
		float64(diskInfo.Used)/1024/1024/1024,
		float64(diskInfo.Total)/1024/1024/1024,
		diskInfo.UsedPercent))

	// 网络流量信息
	netInfo := m.getNetworkInfo()
	buf.WriteString(fmt.Sprintf("📊 *网络流量*: ↓%.2fGB ↑%.2fGB\n", netInfo.RecvGB, netInfo.SentGB))

	// 系统信息
	hostInfo := m.getHostInfo()
	buf.WriteString(fmt.Sprintf("\n🖥️ *系统信息*:\n"))
	buf.WriteString(fmt.Sprintf("• 系统: %s\n", hostInfo.Platform))
	buf.WriteString(fmt.Sprintf("• 运行时间: %s\n", m.formatUptime(hostInfo.Uptime)))

	// 自定义信息内容
	buf.WriteString("\n")
	buf.WriteString(m.config.CustomMessage)
	buf.WriteString("\n")
	
	return buf.String()
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
	// IPv4 处理
	if strings.Count(ip, ".") == 3 {
		parts := strings.Split(ip, ".")
		return "x.x.x." + parts[3]
	}
	// IPv6 处理，仅显示最后8位（去掉分隔符）
	if strings.Contains(ip, ":") {
		// 去除冒号，取8位
		ipStripped := strings.ReplaceAll(ip, ":", "")
		if len(ipStripped) > 8 {
			return "..." + ipStripped[len(ipStripped)-8:]
		}
		return "..." + ipStripped
	}
	// 其它情况直接返回
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

// checkAndSendAlert 检查并发送告警
func (m *ServerMonitor) checkAndSendAlert() {
	var alerts []string

	// 检查 CPU
	cpuPercent := m.getCPUUsage()
	if cpuPercent > float64(m.config.CPUThreshold) {
		alerts = append(alerts, fmt.Sprintf("🔴 CPU 使用率过高: %.1f%%", cpuPercent))
	}

	// 检查内存
	memInfo := m.getMemoryInfo()
	if memInfo.UsedPercent > float64(m.config.MemThreshold) {
		alerts = append(alerts, fmt.Sprintf("🔴 内存使用率过高: %.1f%%", memInfo.UsedPercent))
	}

	// 发送告警
	if len(alerts) > 0 {
		alertMsg := fmt.Sprintf("⚠️ *服务器告警通知*\n\n%s\n\n时间: %s",
			strings.Join(alerts, "\n"),
			time.Now().In(time.FixedZone("CST", 8*3600)).Format("2006-01-02 15:04:05"))
		m.sendMessage(alertMsg)
	}
}

// sendMessage 发送消息
func (m *ServerMonitor) sendMessage(message string) {
	_, err := m.bot.Send(&telebot.Chat{ID: m.config.ChatID}, message, &telebot.SendOptions{
		ParseMode: telebot.ModeMarkdown,
		DisableWebPagePreview: true, // 关闭链接预览
	})
	if err != nil {
		log.Printf("发送消息失败: %v", err)
	}
}