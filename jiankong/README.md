# 服务器监控 Telegram 机器人

一个轻量级的服务器监控程序，通过 Telegram 机器人定时上报服务器状态，支持实时查询和告警通知。

## 功能特性

- 🕐 **定时报告**: 每天指定时间（默认下午3点）自动上报服务器状态
- 📊 **实时监控**: 监控 CPU、内存、磁盘、网络使用情况
- ⚠️ **智能告警**: CPU/内存使用率超过阈值自动发送告警
- 🌍 **地理位置**: 自动获取服务器 IP 和地理位置信息
- 🤖 **交互式界面**: 内置按钮支持实时查询
- 🔧 **灵活配置**: 支持环境变量和 JSON 配置文件
- 🐳 **容器化部署**: 支持 Docker 和 Docker Compose

## 监控指标

- **CPU 使用率**: 实时 CPU 占用百分比
- **内存使用**: 总量/已用量和使用百分比
- **磁盘使用**: 根分区使用情况和百分比
- **网络流量**: 累计上传/下载流量（GB）
- **系统信息**: 操作系统、运行时间
- **地理位置**: 服务器 IP 和所在地区

## 快速开始

### 1. 创建 Telegram 机器人

1. 在 Telegram 中找到 [@BotFather](https://t.me/BotFather)
2. 发送 `/newbot` 创建新机器人
3. 按提示设置机器人名称和用户名
4. 获取 Bot Token

### 2. 获取 Chat ID

1. 将机器人添加到群组或私聊
2. 发送任意消息给机器人
3. 访问 `https://api.telegram.org/bot<YOUR_BOT_TOKEN>/getUpdates`
4. 在返回的 JSON 中找到 `chat.id`

### 3. 配置方式

#### 方式一：环境变量配置

```bash
export BOT_TOKEN="your_bot_token_here"
export CHAT_ID="-1001234567890"
export REPORT_TIME="15:00"
export CUSTOM_MESSAGE="🖥️ 我的服务器状态报告"
export CPU_THRESHOLD="80"
export MEM_THRESHOLD="80"
```

#### 方式二：JSON 配置文件

创建 `config.json` 文件：

```json
{
  "bot_token": "your_bot_token_here",
  "chat_id": 1001234567890,
  "report_time": "15:00",
  "custom_message": "🖥️ 我的服务器状态报告",
  "cpu_threshold": 80,
  "mem_threshold": 80
}
```

### 4. 部署方式

#### 方式一：直接运行

```bash
cd jiankong

# 安装依赖
go mod tidy

# 运行程序
go run main.go
```

#### 方式二：Docker 部署

```bash
# 构建镜像
docker build -t jiankong .

# 运行容器
docker run -d \
  --name server-monitor \
  --restart unless-stopped \
  --privileged \
  --pid host \
  --network host \
  -e BOT_TOKEN="your_bot_token_here" \
  -e CHAT_ID="-1001234567890" \
  -e REPORT_TIME="15:00" \
  -e CUSTOM_MESSAGE="🖥️ Docker 服务器状态报告" \
  -e CPU_THRESHOLD="80" \
  -e MEM_THRESHOLD="80" \
  -v /proc:/host/proc:ro \
  -v /sys:/host/sys:ro \
  -v /:/rootfs:ro \
  jiankong
```

#### 方式三：Docker Compose 部署

1. 修改 `docker-compose.yml` 中的环境变量
2. 运行：

```bash
docker-compose up -d
```

## 配置说明

| 配置项 | 环境变量 | 默认值 | 说明 |
|--------|----------|--------|------|
| Bot Token | `BOT_TOKEN` | 必填 | Telegram 机器人令牌 |
| Chat ID | `CHAT_ID` | 必填 | 接收消息的聊天 ID |
| 报告时间 | `REPORT_TIME` | `15:00` | 每日报告时间（24小时制） |
| 自定义消息 | `CUSTOM_MESSAGE` | `🖥️ 服务器状态报告` | 报告标题 |
| CPU 阈值 | `CPU_THRESHOLD` | `80` | CPU 告警阈值（百分比） |
| 内存阈值 | `MEM_THRESHOLD` | `80` | 内存告警阈值（百分比） |

## 使用说明

### 机器人命令

- `/start` - 启动机器人并显示配置信息
- 点击 "📊 实时状态" 按钮 - 获取当前服务器状态

### 报告内容

机器人会以 Markdown 格式发送包含以下信息的报告：

```
🖥️ 服务器状态报告
🌍 服务器位置: US (1.2.3.4)
🕐 更新时间: 2025-06-26 15:00:05

💚 CPU 使用率: 25.5%
💚 内存使用: 2.1GB/8.0GB (26.3%)
💚 磁盘使用: 45.2GB/100.0GB (45.2%)
📊 网络流量: ↓125.67GB ↑89.34GB

🖥️ 系统信息:
• 系统: linux
• 运行时间: 15天8小时32分钟
```