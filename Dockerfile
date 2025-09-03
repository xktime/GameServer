FROM golang:1.24-alpine AS builder

WORKDIR /app

# 设置镜像源
ENV GOPROXY=https://mirrors.aliyun.com/goproxy/,direct

# 复制 go.mod 和 go.sum 并下载依赖
COPY go.mod go.sum ./

RUN go mod download

# 复制项目源代码
COPY . .

# 构建可执行文件
RUN go build -o GameServer main.go

# 创建更小的运行镜像
FROM alpine:latest


WORKDIR /app

ENV TZ=Asia/Shanghai

# 只复制编译好的二进制文件
COPY --from=builder /app/GameServer .
COPY --from=builder /app/conf ./conf

# websocket端口
EXPOSE 3653
# tcp端口
EXPOSE 3563

# 健康检查
HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 \
  CMD nc -z localhost 3563 || exit 1

# 启动服务
CMD ["./GameServer"]