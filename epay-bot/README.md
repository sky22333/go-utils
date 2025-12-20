## Epay-CDC 支付订单实时通知机器人

- **实时推送**：毫秒级响应，用户付款后立即收到通知。
- **精准不漏**：基于数据库变更日志，高并发下绝不漏单。
- **资源占用低**：内存自动清理，CPU 占用极低。
- **部署简单**：单文件运行，支持多平台 (Windows/Linux/macOS)。

## 部署步骤

### 1. 数据库权限准备

由于程序需要监听 Binlog，数据库用户需要拥有 `REPLICATION CLIENT` 和 `REPLICATION SLAVE` 权限。

```sql
-- 1. 创建用户 (请修改密码)
CREATE USER 'epay_bot'@'%' IDENTIFIED BY 'your_password';

-- 2. 授权
GRANT REPLICATION CLIENT, REPLICATION SLAVE, SELECT ON *.* TO 'epay_bot'@'%';

-- 3. 刷新
FLUSH PRIVILEGES;
```

### 配置说明

```toml
# MySQL 数据库配置
[mysql]
host     = "127.0.0.1"      # 数据库地址
port     = 3306             # 端口
user     = "epay_bot"       # 刚才创建的用户(或者直接使用root用户)
password = "your_password"  # 密码
database = "epay"           # 数据库名
table    = "pay_order"      # 订单表名

# Telegram 机器人配置
[telegram]
bot_token = "123456:ABC-DEF..." # TG Bot Token
chat_id   = "123456789"         # 你的 Chat ID

[binlog]
flavor = "mysql" # 数据库类型 (mysql / mariadb)
```

### 启动运行

**直接运行：**
```bash
./epay_bot
```

**指定配置文件路径：**
```bash
./epay_bot -c /etc/epay/config.toml
```

## 后台运行

推荐使用 Systemd 管理进程，开机自启且自动重启。

1. 创建服务文件 `/etc/systemd/system/epay_bot.service`：

```ini
[Unit]
Description=Epay CDC Bot
After=network.target

[Service]
# 修改为你的程序路径
ExecStart=/opt/epay/epay_bot -c /opt/epay/config.toml
WorkingDirectory=/opt/epay
Restart=always
User=root

[Install]
WantedBy=multi-user.target
```

2. 启动服务：

```bash
systemctl daemon-reload
systemctl enable epay_bot
systemctl start epay_bot
```

3. 常用管理命令：

```bash
# 查看运行状态
systemctl status epay_bot

# 重启服务 (修改配置后需要重启)
systemctl restart epay_bot

# 停止服务
systemctl stop epay_bot

# 禁用开机自启
systemctl disable epay_bot

# 查看实时日志 
journalctl -u epay_bot -f
```

## ❓ 常见问题

**Q: 启动报错 `Access denied; you need (at least one of) the SUPER, REPLICATION CLIENT...`**  
A: 数据库账号权限不足，请参考第一步“数据库权限准备”进行授权。

**Q: 为什么没有收到通知？**  
A: 
1. 检查 `config.toml` 中的 `chat_id` 是否正确。
2. 确保服务器能访问 Telegram API (`api.telegram.org`)。国内服务器可能需要配置代理环境变量。