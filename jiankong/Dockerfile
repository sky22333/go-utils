# 使用官方 Go 1.24 作为构建镜像
FROM golang:1.24-alpine AS builder

# 设置工作目录
WORKDIR /app

# 安装 git 和 ca-certificates（用于 HTTPS 请求）
RUN apk --no-cache add git ca-certificates

# 复制 go mod 和 sum 文件
COPY go.mod go.sum ./

# 下载依赖
RUN go mod download

# 复制源代码
COPY main.go ./

# 构建应用
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main .

# 使用轻量级的 alpine 作为运行镜像
FROM alpine:latest

# 安装 ca-certificates
RUN apk --no-cache add ca-certificates tzdata

# 设置时区为上海（东八区）
RUN cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime && echo "Asia/Shanghai" > /etc/timezone

WORKDIR /root/

# 从构建镜像复制可执行文件
COPY --from=builder /app/main .

# 运行应用
CMD ["./main"]