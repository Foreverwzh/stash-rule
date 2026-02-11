# Stash Rule Service (Go Version)

已将 Stash 配置生成服务重写为 Go 版本，镜像大小约为 **10-15MB**，资源占用极低。

## 🚀 快速开始

### 1. 构建 Docker 镜像

```bash
docker build -t stash-rule:latest .
```

### 2. 本地运行

服务依赖 Redis 存储配置。

1. 启动 Redis:

```bash
docker run -d -p 6379:6379 --name redis redis:alpine
```

2. 启动服务:

```bash
# 方式一：直接运行（读取 .env）
# 环境变量配置 Redis 地址（默认 localhost:6379）
# export REDIS_ADDR="localhost:6379"
go run ./cmd/stash-rule
# 或者编译后运行
# go build -o stash-rule ./cmd/stash-rule && ./stash-rule
```

访问管理页面配置订阅链接与订阅用户: `http://localhost:8080/admin`
获取配置: `http://localhost:8080/?token=<订阅用户token>`（管理员已登录时也可直接访问 `/`）

**默认登录账号**:

- 用户名: `admin`
- 密码: `admin`

**Stash 订阅链接**:

- 登录管理页面后，在“订阅用户管理”中新增订阅用户并复制链接。
- 格式: `http://<your-ip>:8080/?token=<your-token>`
- Token 由服务端随机生成（32 字节随机值的十六进制字符串）。

Docker 运行:

```bash
docker build -t stash-rule:latest .
docker run -d \
  -p 8080:8080 \
  -e REDIS_ADDR="192.168.1.100:6379" \
  --name stash-rule \
  stash-rule:latest
```

### 3. K3s 部署

修改 `deploy/deployment.yaml` 中的 `REDIS_ADDR` 为实际 Redis 服务地址（如 `redis-service:6379` 或 IP），然后应用：

```bash
kubectl apply -f deploy/deployment.yaml
```

- 配置下载: `http://localhost:8080/?token=<订阅用户token>`
- 健康检查: `http://localhost:8080/health`

修改 `deploy/deployment.yaml` 中的环境变量，然后应用：

```bash
kubectl apply -f deploy/deployment.yaml
```

## 📂 文件结构

- `main.go`: HTTP 服务入口
- `config.go`: 配置加载
- `subscriber.go`: 订阅获取与解析
- `stash_config.go`: Stash 配置文件生成逻辑
- `Dockerfile`: 多阶段构建定义

## 🧹 清理指南

确认 Go 版本运行正常后，可安全删除以下 Python 文档：

- `app/` 目录
- `pyproject.toml`
- `uv.lock`
- `.venv/` 目录
