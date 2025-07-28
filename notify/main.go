package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/pelletier/go-toml/v2"
	"golang.org/x/net/proxy"
	tb "gopkg.in/telebot.v3"
)

type Config struct {
	TGBotToken    string `toml:"tg_bot_token"`
	TGAdminID     int64  `toml:"tg_admin_id"`
	PostAuthToken string `toml:"post_auth_token"`
	ListenAddr    string `toml:"listen_addr"`
	DataFile      string `toml:"data_file"`
	MaxFileSize   int64  `toml:"max_file_size"`
	SocksProxy    string `toml:"socks_proxy"`
}

type ReportData struct {
	Time    string                 `json:"time"`
	Content map[string]interface{} `json:"content"`
}

var (
	cfg        Config
	bot        *tb.Bot
	dataFile   string
	dataMu     sync.RWMutex
	userStates sync.Map
	httpServer *http.Server
)

func loadConfig(path string) {
	file, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("读取配置文件失败: %v", err)
	}
	if err := toml.Unmarshal(file, &cfg); err != nil {
		log.Fatalf("解析配置失败: %v", err)
	}
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = ":7000"
	}
	if cfg.DataFile == "" {
		cfg.DataFile = "data.json"
	}
	if cfg.MaxFileSize == 0 {
		cfg.MaxFileSize = 200 // 默认200MB
	}
	dataFile = cfg.DataFile
}

// 创建SOCKS代理客户端
func createProxyClient(proxyAddr string) (*http.Client, error) {
	// 解析代理地址
	proxyURL, err := url.Parse("socks5://" + proxyAddr)
	if err != nil {
		return nil, fmt.Errorf("解析代理地址失败: %v", err)
	}

	// 创建SOCKS5拨号器
	dialer, err := proxy.FromURL(proxyURL, proxy.Direct)
	if err != nil {
		return nil, fmt.Errorf("创建SOCKS5拨号器失败: %v", err)
	}

	// 创建自定义传输层
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.Dial(network, addr)
		},
		MaxIdleConns:        100,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}

	// 创建HTTP客户端
	client := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}

	return client, nil
}

func saveToFile(data ReportData) error {
	dataMu.Lock()
	defer dataMu.Unlock()

	// 检查文件大小，如果超过限制则自动清理旧数据
	if err := checkAndRotateFile(); err != nil {
		log.Printf("文件轮转失败: %v", err)
	}

	f, err := os.OpenFile(dataFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = f.Write(append(jsonData, '\n'))
	return err
}

// 检查文件大小并自动清理
func checkAndRotateFile() error {
	stat, err := os.Stat(dataFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	maxSize := cfg.MaxFileSize * 1024 * 1024 // 转换为字节
	if stat.Size() > maxSize {
		log.Printf("数据文件大小超过限制(%dMB)，开始自动清理旧数据", cfg.MaxFileSize)

		// 清理100MB的旧数据
		return cleanOldDataBySize(100 * 1024 * 1024) // 清理100MB
	}
	return nil
}

// 按大小清理旧数据
func cleanOldDataBySize(targetCleanSize int64) error {
	data, err := loadAllDataUnsafe()
	if err != nil {
		return err
	}

	if len(data) == 0 {
		return nil
	}

	var cleanedSize int64
	var remaining []ReportData
	deletedCount := 0

	// 从最旧的数据开始删除，直到清理了目标大小
	for i, r := range data {
		jsonData, err := json.Marshal(r)
		if err != nil {
			remaining = append(remaining, r)
			continue
		}

		recordSize := int64(len(jsonData) + 1) // +1 for newline

		if cleanedSize < targetCleanSize {
			cleanedSize += recordSize
			deletedCount++
		} else {
			// 保留剩余的数据
			remaining = append(remaining, data[i:]...)
			break
		}
	}

	// 重写文件
	f, err := os.OpenFile(dataFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	// 使用缓冲写入提高性能
	writer := bufio.NewWriter(f)
	defer writer.Flush()

	for _, r := range remaining {
		jsonData, err := json.Marshal(r)
		if err != nil {
			continue
		}
		writer.Write(append(jsonData, '\n'))
	}

	log.Printf("清理了 %d 条旧数据(约%.2fMB)，剩余 %d 条数据", deletedCount, float64(cleanedSize)/(1024*1024), len(remaining))
	return nil
}

func loadAllData() ([]ReportData, error) {
	dataMu.RLock()
	defer dataMu.RUnlock()

	f, err := os.Open(dataFile)
	if err != nil {
		if os.IsNotExist(err) {
			return []ReportData{}, nil // 文件不存在返回空切片
		}
		return nil, err
	}
	defer f.Close()

	var results []ReportData
	scanner := bufio.NewScanner(f)
	// 增加缓冲区大小以处理大文件
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		var r ReportData
		line := scanner.Bytes()
		if err := json.Unmarshal(line, &r); err == nil {
			results = append(results, r)
		}
	}
	return results, scanner.Err()
}

func loadRecentData(limit int) ([]ReportData, error) {
	data, err := loadAllData()
	if err != nil {
		return nil, err
	}

	// 返回最近的数据（倒序取前limit条）
	if len(data) <= limit {
		return data, nil
	}
	return data[len(data)-limit:], nil
}

func deleteDataByTimeRange(start, end time.Time) error {
	// 注意：这里不加锁，因为调用方已经加锁了
	data, err := loadAllDataUnsafe()
	if err != nil {
		return err
	}

	var remaining []ReportData
	deletedCount := 0

	for _, r := range data {
		t, err := time.Parse(time.RFC3339, r.Time)
		if err != nil {
			remaining = append(remaining, r) // 保留解析失败的数据
			continue
		}
		// 如果时间不在删除范围内，保留
		if start.IsZero() || end.IsZero() || t.Before(start) || t.After(end) {
			remaining = append(remaining, r)
		} else {
			deletedCount++
		}
	}

	// 重写文件
	f, err := os.OpenFile(dataFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	// 使用缓冲写入提高性能
	writer := bufio.NewWriter(f)
	defer writer.Flush()

	for _, r := range remaining {
		jsonData, err := json.Marshal(r)
		if err != nil {
			continue
		}
		writer.Write(append(jsonData, '\n'))
	}

	log.Printf("删除了 %d 条数据，剩余 %d 条数据", deletedCount, len(remaining))
	return nil
}

// 不加锁的数据读取，用于已经加锁的场景
func loadAllDataUnsafe() ([]ReportData, error) {
	f, err := os.Open(dataFile)
	if err != nil {
		if os.IsNotExist(err) {
			return []ReportData{}, nil
		}
		return nil, err
	}
	defer f.Close()

	var results []ReportData
	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		var r ReportData
		line := scanner.Bytes()
		if err := json.Unmarshal(line, &r); err == nil {
			results = append(results, r)
		}
	}
	return results, scanner.Err()
}

func filterDataByTime(data []ReportData, start, end time.Time) []ReportData {
	var filtered []ReportData
	for _, r := range data {
		t, err := time.Parse(time.RFC3339, r.Time)
		if err != nil {
			continue
		}
		if !t.Before(start) && !t.After(end) {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

func formatDataMessage(data []ReportData) string {
	if len(data) == 0 {
		return "暂无数据"
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("📊 共找到 %d 条数据\n\n", len(data)))

	displayCount := len(data)
	if displayCount > 20 {
		displayCount = 20
	}

	for i := 0; i < displayCount; i++ {
		r := data[i]
		b.WriteString(fmt.Sprintf("🕐 %s\n", r.Time))
		j, _ := json.MarshalIndent(r.Content, "", "  ")
		b.Write(j)
		b.WriteString("\n\n")
	}

	if len(data) > 20 {
		b.WriteString(fmt.Sprintf("... 还有 %d 条数据，请缩小时间范围查询\n", len(data)-20))
	}

	return b.String()
}

func sendToAdmin(data ReportData) {
	// 异步发送，避免阻塞HTTP请求
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("发送TG消息时发生panic: %v", r)
			}
		}()

		var buf bytes.Buffer
		buf.WriteString(fmt.Sprintf("📥 收到新数据 (%s)\n\n", data.Time))

		enc := json.NewEncoder(&buf)
		enc.SetIndent("", "  ")
		_ = enc.Encode(data.Content)

		admin := &tb.User{ID: cfg.TGAdminID}

		// 添加超时控制
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		done := make(chan error, 1)
		go func() {
			_, err := bot.Send(admin, buf.String())
			done <- err
		}()

		select {
		case err := <-done:
			if err != nil {
				log.Printf("发送 TG 消息失败: %v", err)
			}
		case <-ctx.Done():
			log.Printf("发送 TG 消息超时")
		}
	}()
}

func handler(w http.ResponseWriter, r *http.Request) {
	// 添加请求大小限制
	r.Body = http.MaxBytesReader(w, r.Body, 10*1024*1024) // 10MB限制

	if r.Method != http.MethodPost {
		http.Error(w, "只支持 POST 请求", http.StatusMethodNotAllowed)
		return
	}

	authHeader := r.Header.Get("Authorization")
	expectedAuth := "Bearer " + cfg.PostAuthToken
	if authHeader != expectedAuth {
		http.Error(w, "认证失败", http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "读取请求失败", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var content map[string]interface{}
	if err := json.Unmarshal(body, &content); err != nil {
		http.Error(w, "JSON格式错误", http.StatusBadRequest)
		return
	}

	loc, _ := time.LoadLocation("Asia/Shanghai")
	report := ReportData{
		Time:    time.Now().In(loc).Format(time.RFC3339),
		Content: content,
	}

	if err := saveToFile(report); err != nil {
		log.Printf("保存失败: %v", err)
		http.Error(w, "服务器错误", http.StatusInternalServerError)
		return
	}

	sendToAdmin(report)
	log.Println("✅ 数据接收成功")

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}

// 权限检查函数
func isAdmin(userID int64) bool {
	return userID == cfg.TGAdminID
}

func setupBotHandlers() {
	startBtn := tb.InlineButton{Unique: "start_btn", Text: "📊 最近数据"}
	queryBtn := tb.InlineButton{Unique: "query_btn", Text: "🔍 时间查询"}
	cleanBtn := tb.InlineButton{Unique: "clean_btn", Text: "🗑️ 清理数据"}
	statusBtn := tb.InlineButton{Unique: "status_btn", Text: "📈 系统状态"}
	backBtn := tb.InlineButton{Unique: "back_btn", Text: "🔙 返回主菜单"}

	bot.Handle("/start", func(c tb.Context) error {
		if !isAdmin(c.Sender().ID) {
			return c.Send("❌ 权限不足，仅管理员可使用此机器人")
		}
		userStates.Delete(c.Sender().ID) // 清除用户状态
		markup := &tb.ReplyMarkup{
			InlineKeyboard: [][]tb.InlineButton{
				{startBtn, queryBtn},
				{cleanBtn, statusBtn},
			},
		}
		return c.Send("🤖 欢迎使用数据查询机器人！\n\n请选择操作：", markup)
	})

	bot.Handle(&statusBtn, func(c tb.Context) error {
		if !isAdmin(c.Sender().ID) {
			return c.Edit("❌ 权限不足")
		}
		data, err := loadAllData()
		if err != nil {
			return c.Edit("❌ 读取数据失败：" + err.Error())
		}

		var m runtime.MemStats
		runtime.ReadMemStats(&m)

		stat, _ := os.Stat(dataFile)
		fileSize := int64(0)
		if stat != nil {
			fileSize = stat.Size()
		}

		status := fmt.Sprintf(`📈 系统状态

📊 数据统计：
• 总记录数：%d 条
• 文件大小：%.2f MB
• 最大限制：%d MB

💾 内存使用：
• 当前内存：%.2f MB
• 系统内存：%.2f MB

⚙️ 配置信息：
• 监听地址：%s
• 数据文件：%s`,
			len(data),
			float64(fileSize)/(1024*1024),
			cfg.MaxFileSize,
			float64(m.Alloc)/(1024*1024),
			float64(m.Sys)/(1024*1024),
			cfg.ListenAddr,
			cfg.DataFile)

		markup := &tb.ReplyMarkup{
			InlineKeyboard: [][]tb.InlineButton{{backBtn}},
		}
		return c.Edit(status, markup)
	})

	bot.Handle(&backBtn, func(c tb.Context) error {
		if !isAdmin(c.Sender().ID) {
			return c.Edit("❌ 权限不足")
		}
		userStates.Delete(c.Sender().ID)
		markup := &tb.ReplyMarkup{
			InlineKeyboard: [][]tb.InlineButton{
				{startBtn, queryBtn},
				{cleanBtn, statusBtn},
			},
		}
		return c.Edit("🤖 欢迎使用数据查询机器人！\n\n请选择操作：", markup)
	})

	bot.Handle(&startBtn, func(c tb.Context) error {
		if !isAdmin(c.Sender().ID) {
			return c.Edit("❌ 权限不足")
		}
		data, err := loadRecentData(20)
		if err != nil {
			markup := &tb.ReplyMarkup{
				InlineKeyboard: [][]tb.InlineButton{{backBtn}},
			}
			return c.Edit("❌ 读取数据失败："+err.Error(), markup)
		}
		if len(data) == 0 {
			markup := &tb.ReplyMarkup{
				InlineKeyboard: [][]tb.InlineButton{{backBtn}},
			}
			return c.Edit("📭 暂无数据", markup)
		}

		msg := formatDataMessage(data)
		markup := &tb.ReplyMarkup{
			InlineKeyboard: [][]tb.InlineButton{{backBtn}},
		}
		return c.Edit(msg, markup)
	})

	bot.Handle(&queryBtn, func(c tb.Context) error {
		if !isAdmin(c.Sender().ID) {
			return c.Edit("❌ 权限不足")
		}
		userStates.Store(c.Sender().ID, "query")
		markup := &tb.ReplyMarkup{
			InlineKeyboard: [][]tb.InlineButton{{backBtn}},
		}
		return c.Edit("🔍 请输入查询时间段（精确到天）\n\n格式：`YYYY-MM-DD ~ YYYY-MM-DD`\n例如：`2025-07-01 ~ 2025-07-28`", &tb.SendOptions{
			ParseMode:   tb.ModeMarkdown,
			ReplyMarkup: markup,
		})
	})

	bot.Handle(&cleanBtn, func(c tb.Context) error {
		if !isAdmin(c.Sender().ID) {
			return c.Edit("❌ 权限不足")
		}
		userStates.Store(c.Sender().ID, "clean")
		markup := &tb.ReplyMarkup{
			InlineKeyboard: [][]tb.InlineButton{{backBtn}},
		}
		return c.Edit("🗑️ 请输入要清理的时间段（精确到天）\n\n格式：`YYYY-MM-DD ~ YYYY-MM-DD`\n例如：`2025-07-01 ~ 2025-07-28`\n\n⚠️ 注意：此操作将永久删除指定时间段内的数据！", &tb.SendOptions{
			ParseMode:   tb.ModeMarkdown,
			ReplyMarkup: markup,
		})
	})

	bot.Handle(tb.OnText, func(c tb.Context) error {
		if !isAdmin(c.Sender().ID) {
			return c.Send("❌ 权限不足，仅管理员可使用此机器人")
		}
		
		txt := strings.TrimSpace(c.Text())
		userID := c.Sender().ID

		// 检查用户状态
		state, exists := userStates.Load(userID)
		if !exists || !strings.Contains(txt, "~") {
			return c.Send("❓ 请先选择操作类型，查看菜单 /start")
		}

		parts := strings.SplitN(txt, "~", 2)
		if len(parts) != 2 {
			return c.Send("❌ 时间格式错误\n\n格式示例：`YYYY-MM-DD ~ YYYY-MM-DD`", &tb.SendOptions{ParseMode: tb.ModeMarkdown})
		}

		layout := "2006-01-02"
		startDateStr := strings.TrimSpace(parts[0])
		endDateStr := strings.TrimSpace(parts[1])

		start, err1 := time.ParseInLocation(layout, startDateStr, time.Local)
		end, err2 := time.ParseInLocation(layout, endDateStr, time.Local)
		if err1 != nil || err2 != nil {
			return c.Send("❌ 时间格式错误\n\n格式示例：`YYYY-MM-DD ~ YYYY-MM-DD`", &tb.SendOptions{ParseMode: tb.ModeMarkdown})
		}

		if start.After(end) {
			return c.Send("❌ 开始时间不能晚于结束时间")
		}

		// 结束时间扩展至当天23:59:59，包含全天数据
		end = end.Add(23*time.Hour + 59*time.Minute + 59*time.Second)

		switch state {
		case "clean":
			// 清理数据操作
			confirmBtn := tb.InlineButton{Unique: "confirm_clean_" + startDateStr + "_" + endDateStr, Text: "✅ 确认删除"}
			cancelBtn := tb.InlineButton{Unique: "cancel_clean", Text: "❌ 取消"}

			markup := &tb.ReplyMarkup{
				InlineKeyboard: [][]tb.InlineButton{{confirmBtn, cancelBtn}},
			}

			userStates.Delete(userID) // 清除状态
			return c.Send(fmt.Sprintf("⚠️ 确认要删除 %s 到 %s 时间段内的所有数据吗？\n\n🚨 此操作不可撤销！", startDateStr, endDateStr), markup)

		case "query":
			// 查询数据操作
			data, err := loadAllData()
			if err != nil {
				return c.Send("❌ 读取数据失败：" + err.Error())
			}
			filtered := filterDataByTime(data, start, end)
			if len(filtered) == 0 {
				markup := &tb.ReplyMarkup{
					InlineKeyboard: [][]tb.InlineButton{{backBtn}},
				}
				return c.Send("📭 该时间段内无数据", markup)
			}

			msg := formatDataMessage(filtered)
			markup := &tb.ReplyMarkup{
				InlineKeyboard: [][]tb.InlineButton{{backBtn}},
			}
			userStates.Delete(userID) // 清除状态
			return c.Send(msg, markup)
		}

		return nil
	})

	// 处理清理确认按钮
	bot.Handle(tb.OnCallback, func(c tb.Context) error {
		if !isAdmin(c.Sender().ID) {
			return c.Edit("❌ 权限不足")
		}
		
		data := c.Callback().Data
		if strings.HasPrefix(data, "confirm_clean_") {
			// 解析时间范围
			parts := strings.TrimPrefix(data, "confirm_clean_")
			dateParts := strings.Split(parts, "_")
			if len(dateParts) != 2 {
				return c.Edit("❌ 操作失败：时间解析错误")
			}

			layout := "2006-01-02"
			start, err1 := time.ParseInLocation(layout, dateParts[0], time.Local)
			end, err2 := time.ParseInLocation(layout, dateParts[1], time.Local)
			if err1 != nil || err2 != nil {
				return c.Edit("❌ 操作失败：时间格式错误")
			}

			// 结束时间扩展至当天23:59:59
			end = end.Add(23*time.Hour + 59*time.Minute + 59*time.Second)

			// 执行删除
			dataMu.Lock()
			err := deleteDataByTimeRange(start, end)
			dataMu.Unlock()

			if err != nil {
				markup := &tb.ReplyMarkup{
					InlineKeyboard: [][]tb.InlineButton{{backBtn}},
				}
				return c.Edit("❌ 清理失败："+err.Error(), markup)
			}

			markup := &tb.ReplyMarkup{
				InlineKeyboard: [][]tb.InlineButton{{backBtn}},
			}
			return c.Edit(fmt.Sprintf("✅ 已成功清理 %s 到 %s 时间段内的数据", dateParts[0], dateParts[1]), markup)
		} else if data == "cancel_clean" {
			markup := &tb.ReplyMarkup{
				InlineKeyboard: [][]tb.InlineButton{{backBtn}},
			}
			return c.Edit("❌ 已取消清理操作", markup)
		}
		return nil
	})
}

func main() {
	loadConfig("config.toml")

	// 设置机器人配置
	pref := tb.Settings{
		Token: cfg.TGBotToken,
		Poller: &tb.LongPoller{
			Timeout:        10 * time.Second,
			AllowedUpdates: []string{"message", "callback_query"},
		},
	}

	// 如果配置了SOCKS代理，则设置代理客户端
	if cfg.SocksProxy != "" {
		client, err := createProxyClient(cfg.SocksProxy)
		if err != nil {
			log.Fatalf("创建代理客户端失败: %v", err)
		}
		pref.Client = client
		log.Printf("🌐 已配置SOCKS代理: %s", cfg.SocksProxy)
	}

	var err error
	bot, err = tb.NewBot(pref)
	if err != nil {
		log.Fatalf("启动 Telegram 机器人失败: %v", err)
	}

	setupBotHandlers()

	// 启动HTTP服务器
	mux := http.NewServeMux()
	mux.HandleFunc("/report", handler)

	httpServer = &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("🌐 启动 HTTP 服务监听: %s", cfg.ListenAddr)
		log.Printf("📡 接口地址: %s/report", cfg.ListenAddr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP 服务异常退出: %v", err)
		}
	}()

	// 启动定期清理任务
	go func() {
		ticker := time.NewTicker(24 * time.Hour) // 每天检查一次
		defer ticker.Stop()

		for range ticker.C {
			log.Println("🧹 开始定期数据清理检查...")
			if err := checkAndRotateFile(); err != nil {
				log.Printf("定期清理失败: %v", err)
			}
			runtime.GC() // 手动触发垃圾回收
		}
	}()

	log.Println("🤖 Telegram 机器人启动中...")
	log.Printf("👤 管理员ID: %d", cfg.TGAdminID)
	log.Printf("📁 数据文件: %s", cfg.DataFile)
	log.Printf("🗂️ 最大文件大小: %dMB", cfg.MaxFileSize)
	if cfg.SocksProxy != "" {
		log.Printf("🔗 SOCKS代理: %s", cfg.SocksProxy)
	}

	bot.Start()
}
