package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/go-mysql-org/go-mysql/canal"
	"github.com/go-mysql-org/go-mysql/schema"
	_ "github.com/go-sql-driver/mysql"
	"github.com/pelletier/go-toml/v2"
)

// Config 配置结构
type Config struct {
	MySQL struct {
		Host     string `toml:"host"`
		Port     int    `toml:"port"`
		User     string `toml:"user"`
		Password string `toml:"password"`
		Database string `toml:"database"`
		Table    string `toml:"table"`
	} `toml:"mysql"`

	Telegram struct {
		BotToken string `toml:"bot_token"`
		ChatID   string `toml:"chat_id"`
	} `toml:"telegram"`

	Binlog struct {
		Flavor string `toml:"flavor"`
	} `toml:"binlog"`
}

var cfg Config

// PayTypeCache 支付方式缓存
type PayTypeCache struct {
	sync.RWMutex
	mapping map[string]string
	dsn     string // 保存 DSN 用于惰性加载
}

func NewPayTypeCache() *PayTypeCache {
	return &PayTypeCache{mapping: make(map[string]string)}
}

// Load 加载支付方式
func (p *PayTypeCache) Load(dsn string) error {
	p.dsn = dsn // 保存 DSN
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return err
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := db.QueryContext(ctx, "SELECT id, showname FROM pay_type")
	if err != nil {
		return fmt.Errorf("查询 pay_type 失败: %v", err)
	}
	defer rows.Close()

	newMapping := make(map[string]string)
	for rows.Next() {
		var id int
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			continue
		}
		newMapping[strconv.Itoa(id)] = name
	}

	p.Lock()
	p.mapping = newMapping
	p.Unlock()

	log.Printf("已加载 %d 种支付方式", len(newMapping))
	return nil
}

// GetName 获取支付名称 (惰性加载)
func (p *PayTypeCache) GetName(id string) string {
	// 1. 尝试读锁获取
	p.RLock()
	name, ok := p.mapping[id]
	p.RUnlock()
	if ok {
		return name
	}

	// 2. 缓存未命中，从数据库查询
	return p.fetchFromDB(id)
}

// fetchFromDB 从数据库查询单个支付方式
func (p *PayTypeCache) fetchFromDB(id string) string {
	db, err := sql.Open("mysql", p.dsn)
	if err != nil {
		log.Printf("连接数据库失败: %v", err)
		return "未知支付方式"
	}
	defer db.Close()

	var name string
	err = db.QueryRow("SELECT showname FROM pay_type WHERE id = ?", id).Scan(&name)
	if err != nil {
		if err != sql.ErrNoRows {
			log.Printf("查询支付方式失败 ID=%s: %v", id, err)
		}
		return "未知支付方式"
	}

	// 3. 更新缓存
	p.Lock()
	p.mapping[id] = name
	p.Unlock()

	log.Printf("🔄 已自动加载新支付方式: ID=%s Name=%s", id, name)
	return name
}

// NotifyCache 通知缓存 (带 TTL)
type NotifyCache struct {
	sync.Map
}

func NewNotifyCache() *NotifyCache {
	nc := &NotifyCache{}
	go nc.cleanupLoop()
	return nc
}

// MarkNotified 标记已通知
func (nc *NotifyCache) MarkNotified(key string) {
	nc.Store(key, time.Now().Unix())
}

// HasNotified 检查是否已通知
func (nc *NotifyCache) HasNotified(key string) bool {
	_, ok := nc.Load(key)
	return ok
}

// cleanupLoop 清理过期记录 (24小时)
func (nc *NotifyCache) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Hour)
	for range ticker.C {
		now := time.Now().Unix()
		nc.Range(func(key, value interface{}) bool {
			if timestamp, ok := value.(int64); ok {
				if now-timestamp > 86400 {
					nc.Delete(key)
				}
			} else {
				nc.Delete(key)
			}
			return true
		})
	}
}

// EventHandler Binlog 事件处理
type EventHandler struct {
	canal.DummyEventHandler
	payTypeCache *PayTypeCache
	notifyCache  *NotifyCache
}

func (h *EventHandler) OnRow(e *canal.RowsEvent) error {
	if e.Table.Name != cfg.MySQL.Table {
		return nil
	}

	switch e.Action {
	case canal.InsertAction:
		for _, row := range e.Rows {
			h.handleInsert(row, e.Table)
		}
	case canal.UpdateAction:
		for i := 0; i < len(e.Rows); i += 2 {
			h.handleUpdate(e.Rows[i], e.Rows[i+1], e.Table)
		}
	}
	return nil
}

func (h *EventHandler) handleInsert(row []interface{}, table *schema.Table) {
	data := rowToMap(row, table)

	if val, ok := data["status"]; !ok || val != "0" {
		return
	}

	tradeNo := data["trade_no"]
	key := tradeNo + ":0"
	
	if h.notifyCache.HasNotified(key) {
		return
	}
	h.notifyCache.MarkNotified(key)

	payName := h.payTypeCache.GetName(data["type"])
	sendTelegram(fmt.Sprintf(
		"🕒 新订单待支付\n————————————\n订单号：%s\n金额：%s\n商品：%s\n支付方式：%s",
		tradeNo, data["money"], data["name"], payName,
	))
}

func (h *EventHandler) handleUpdate(oldRow, newRow []interface{}, table *schema.Table) {
	oldData := rowToMap(oldRow, table)
	newData := rowToMap(newRow, table)

	if oldData["status"] == "0" && newData["status"] == "1" {
		tradeNo := newData["trade_no"]
		key := tradeNo + ":1"
		
		if h.notifyCache.HasNotified(key) {
			return
		}
		h.notifyCache.MarkNotified(key)

		payName := h.payTypeCache.GetName(newData["type"])
		sendTelegram(fmt.Sprintf(
			"✅ 订单支付完成\n————————————\n订单号：%s\n金额：%s\n商品：%s\n支付方式：%s",
			tradeNo, newData["money"], newData["name"], payName,
		))
	}
}

// rowToMap 将行数据转为 Map
func rowToMap(row []interface{}, table *schema.Table) map[string]string {
	result := make(map[string]string, len(table.Columns))
	for i, col := range table.Columns {
		if i < len(row) {
			val := row[i]
			switch v := val.(type) {
			case []byte:
				result[col.Name] = string(v)
			default:
				result[col.Name] = fmt.Sprint(v)
			}
		}
	}
	return result
}

// sendTelegram 发送通知
func sendTelegram(text string) {
	api := fmt.Sprintf(
		"https://api.telegram.org/bot%s/sendMessage",
		cfg.Telegram.BotToken,
	)

	data := url.Values{}
	data.Set("chat_id", cfg.Telegram.ChatID)
	data.Set("text", text)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.PostForm(api, data)
	if err != nil {
		log.Println("Telegram 发送失败:", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Println("Telegram 返回异常:", resp.Status)
	}
}

// getConfigPath 获取配置文件路径
func getConfigPath() string {
	// 1. 优先使用命令行参数
	configPath := flag.String("c", "", "配置文件路径 (默认: config.toml)")
	flag.Parse()

	if *configPath != "" {
		return *configPath
	}

	// 2. 检查当前目录下的 config.toml
	if _, err := os.Stat("config.toml"); err == nil {
		return "config.toml"
	}

	// 3. 检查可执行文件同级目录
	exePath, err := os.Executable()
	if err == nil {
		exeDir := filepath.Dir(exePath)
		path := filepath.Join(exeDir, "config.toml")
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	// 4. 默认返回 config.toml，让加载函数报错
	return "config.toml"
}

func main() {
	// 1. 加载配置
	configPath := getConfigPath()
	log.Printf("加载配置文件: %s", configPath)
	
	if err := loadConfig(configPath); err != nil {
		log.Fatalf("加载配置失败: %v\n请确保配置文件存在或使用 -c 参数指定", err)
	}

	// 2. 初始化缓存
	payCache := NewPayTypeCache()
	notifyCache := NewNotifyCache()

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		cfg.MySQL.User, cfg.MySQL.Password, cfg.MySQL.Host, cfg.MySQL.Port, cfg.MySQL.Database)
	
	if err := payCache.Load(dsn); err != nil {
		log.Printf("⚠️ 初始化支付方式失败: %v", err)
	}

	// 3. 配置 Canal
	canalCfg := canal.NewDefaultConfig()
	canalCfg.Addr = cfg.MySQL.Host + ":" + strconv.Itoa(cfg.MySQL.Port)
	canalCfg.User = cfg.MySQL.User
	canalCfg.Password = cfg.MySQL.Password
	canalCfg.Flavor = cfg.Binlog.Flavor

	canalCfg.IncludeTableRegex = []string{
		cfg.MySQL.Database + "\\." + cfg.MySQL.Table,
	}

	c, err := canal.NewCanal(canalCfg)
	if err != nil {
		log.Fatal(err)
	}

	// 4. 注册 EventHandler
	c.SetEventHandler(&EventHandler{
		payTypeCache: payCache,
		notifyCache:  notifyCache,
	})

	// 5. 启动服务
	pos, err := c.GetMasterPos()
	if err != nil {
		log.Fatal(err)
	}

	log.Println("🚀 服务已启动，开始监听 Binlog")

	if err := c.RunFrom(pos); err != nil {
		log.Fatal(err)
	}
}

// loadConfig 读取配置
func loadConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return toml.Unmarshal(data, &cfg)
}
