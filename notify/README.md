## Telegram自动推送通知机器人

一个基于Go语言开发的自动推送通知机器人，结合HTTP API和Telegram机器人，用于接收、存储和查询JSON数据并实现自动通知。

## 🚀 主要功能

- **数据接收**: 提供HTTP POST接口接收JSON格式数据
- **实时通知**: 新数据自动推送到Telegram管理员
- **数据查询**: 通过Telegram机器人查看和搜索历史数据
- **数据管理**: 支持按时间范围清理数据，自动文件大小管理
- **代理支持**: 支持SOCKS代理，解决网络访问限制

## 📱 机器人功能

- 📊 查看最近20条数据
- 🔍 按时间范围查询数据
- 🗑️ 按时间范围清理数据
- 📈 查看系统状态和统计信息

## ⚙️ 配置说明

修改 `config.toml` 文件：

```toml
# 你的机器人Token
tg_bot_token = "1234567:AAAAAAAAAAAAAAAAAAAAA"

# 管理员TG用户ID
tg_admin_id = 12345678

# POST接口鉴权Token
post_auth_token = "yourtoken"

# 监听地址及端口，默认7000
listen_addr = ":7000"

# 数据保存文件，json格式
data_file = "data.json"

# 最大文件大小(MB)，默认200MB
max_file_size = 200

# SOCKS代理地址，例如 "127.0.0.1:1080"，留空则不使用代理
socks_proxy = ""
```

## 🔧 快速开始

1. 配置 `config.toml` 文件
2. 启动程序：`go run main.go`
3. 测试接口：`go run test_client.go`

## 📡 接口使用

```bash
curl -X POST http://127.0.0.1:7000/report \
 -H "Authorization: Bearer yourtoken" \
 -H "Content-Type: application/json" \
 -d '{"用户":"张三","操作":"登录系统"}'
```

## 🧪 测试工具

- `test_client.go`: 功能测试  