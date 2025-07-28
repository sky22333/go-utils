FROM golang:1.24-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -trimpath -o notify .

FROM alpine

RUN apk --no-cache add tzdata ca-certificates
ENV TZ=Asia/Shanghai

WORKDIR /root/

COPY --from=builder /app/notify .
COPY --from=builder /app/config.toml .

CMD ["./notify"]
