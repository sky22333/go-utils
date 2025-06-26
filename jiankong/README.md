### 轻量级服务器监控 Telegram 机器人

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

### 2. Telegram账户 ID

访问机器人获取：https://t.me/creationdatebot

### 4. 部署方式

#### 方式一：系统服务运行

```
sudo mkdir -p /opt/jiankong && sudo wget -O /opt/jiankong/jiankong https://raw.githubusercontent.com/sky22333/go-utils/main/jiankong/jiankong && sudo chmod +x /opt/jiankong/jiankong && sudo wget -O /etc/systemd/system/jiankong.service https://raw.githubusercontent.com/sky22333/go-utils/main/jiankong/jiankong.service
```
然后修改`/etc/systemd/system/jiankong.service`配置
```
# 重载系统服务
sudo systemctl daemon-reload

# 开机自启
sudo systemctl enable jiankong

# 启动
sudo systemctl start jiankong

# 重启
sudo systemctl restart jiankong

# 查看运行状态
sudo systemctl status jiankong

# 查看实时日志
sudo journalctl -u jiankong -f

# 停止服务
sudo systemctl stop jiankong
```

#### 方式二：Docker Compose 部署

1. 修改 `docker-compose.yml` 中的环境变量
2. 运行：

```bash
docker-compose up -d
```

## 配置说明

| 配置项 | 环境变量 | 默认值 | 说明 |
|--------|----------|--------|------|
| Bot Token | `BOT_TOKEN` | 必填 | Telegram 机器人令牌 |
| Chat ID | `CHAT_ID` | 必填 | Telegram账户 ID |
| 报告时间 | `REPORT_TIME` | `15:00` | 每日报告时间（24小时制） |
| 自定义消息 | `CUSTOM_MESSAGE` | `🖥️ 服务器状态报告` | 报告标题 |
| CPU 阈值 | `CPU_THRESHOLD` | `80` | CPU 告警阈值（百分比） |
| 内存阈值 | `MEM_THRESHOLD` | `80` | 内存告警阈值（百分比） |

## 使用说明

### 机器人命令

- `/start` - 启动机器人并显示配置信息
- 点击 "📊 实时状态" 按钮 - 获取当前服务器状态

### 报告内容

机器人会以如下格式发送包含以下信息的报告：

```
🖥️ 服务器状态报告
🌍 服务器位置: US (x.x.x.123)
🕐 更新时间: 2025-06-26 15:00:05

💚 CPU 使用率: 2.9%
💚 内存使用: 206.6MB/979.0MB (21.1%)
💚 磁盘使用: 2.5GB/9.8GB (26.3%)
📊 网络流量: ↓0.84GB ↑0.51GB

🖥️ 系统信息:
• 系统: debian
• 运行时间: 15天8小时32分钟
```
