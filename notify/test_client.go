package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// 配置信息
const (
	serverURL = "http://127.0.0.1:7000/report"
	authToken = "yourtoken" // 需要与config.toml中的post_auth_token一致
)

// 测试数据结构
type TestData struct {
	User     string                 `json:"用户"`
	Action   string                 `json:"操作"`
	IP       string                 `json:"IP地址,omitempty"`
	Device   string                 `json:"设备,omitempty"`
	Location map[string]string      `json:"位置,omitempty"`
	Duration int                    `json:"时长,omitempty"`
	Success  bool                   `json:"成功"`
	Extra    map[string]interface{} `json:"额外信息,omitempty"`
}

// 发送数据到服务器
func sendData(data interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("JSON序列化失败: %v", err)
	}

	req, err := http.NewRequest("POST", serverURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("创建请求失败: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+authToken)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("发送请求失败: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取响应失败: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("服务器返回错误 %d: %s", resp.StatusCode, string(body))
	}

	fmt.Printf("✅ 数据发送成功: %s\n", string(body))
	return nil
}

func main() {
	fmt.Println("🚀 开始测试数据提交...")
	fmt.Printf("📡 服务器地址: %s\n", serverURL)
	fmt.Printf("🔑 认证Token: %s\n", authToken)
	fmt.Println()

	// 测试数据集
	testCases := []interface{}{
		// 1. 简单登录数据
		TestData{
			User:    "张三",
			Action:  "登录系统",
			Success: true,
		},

		// 2. 详细操作数据
		TestData{
			User:     "李四",
			Action:   "查询订单",
			IP:       "192.168.1.100",
			Device:   "iPhone 15",
			Duration: 1200,
			Success:  true,
			Location: map[string]string{
				"城市": "北京",
				"区域": "朝阳区",
			},
		},

		// 3. 失败操作数据
		TestData{
			User:    "王五",
			Action:  "删除文件",
			IP:      "10.0.0.50",
			Device:  "Windows PC",
			Success: false,
			Extra: map[string]interface{}{
				"错误代码": 403,
				"错误信息": "权限不足",
				"重试次数": 3,
			},
		},

		// 4. 自定义格式数据
		map[string]interface{}{
			"事件类型":   "系统监控",
			"服务器":    "web-01",
			"CPU使用率": 85.6,
			"内存使用率":  72.3,
			"磁盘空间": map[string]interface{}{
				"总容量": "500GB",
				"已使用": "320GB",
				"剩余":  "180GB",
			},
			"状态":  "警告",
			"时间戳": time.Now().Unix(),
		},

		// 5. 用户行为数据
		map[string]interface{}{
			"用户ID":  "user_12345",
			"页面":    "/dashboard",
			"动作":    "点击按钮",
			"按钮名称":  "导出报表",
			"浏览器":   "Chrome 120",
			"屏幕分辨率": "1920x1080",
			"停留时间":  45,
			"来源页面":  "/reports",
		},
	}

	// 逐个发送测试数据
	for i, testData := range testCases {
		fmt.Printf("📤 发送测试数据 %d/%d...\n", i+1, len(testCases))

		if err := sendData(testData); err != nil {
			log.Printf("❌ 测试数据 %d 发送失败: %v\n", i+1, err)
		} else {
			fmt.Printf("✅ 测试数据 %d 发送成功\n", i+1)
		}

		// 间隔1秒，避免请求过快
		time.Sleep(1 * time.Second)
		fmt.Println()
	}

	fmt.Println("🎉 所有测试数据发送完成！")
	fmt.Println("💡 请检查Telegram机器人是否收到通知")
	fmt.Println("📊 可以通过机器人查看接收到的数据")
}
