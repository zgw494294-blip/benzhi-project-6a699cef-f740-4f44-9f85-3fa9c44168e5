# BENZHI_README

## 项目说明
- 项目：benzhi-project-6a699cef-f740-4f44-9f85-3fa9c44168e5
- 项目用途：Implemented BenchSlot with complete reservation transitions, conflict detection, validated atomic JSON persistence, receipt rendering, smoke workflow, tests, and README documentation. Both acceptance commands pass.
- Go 工具链：`golang:1.22.0`
- 前端工具链：无

## 标准构建、运行和测试命令
进入容器后执行：
cd '/app' && GOTOOLCHAIN=local go build ./...
cd /app && GOTOOLCHAIN=local go run ./cmd/benchslot
cd '/app' && GOTOOLCHAIN=local go test ./...

## Docker 构建和进入容器
chmod +x build_benzhi_docker.sh
./build_benzhi_docker.sh benzhi-project-6a699cef-f740-4f44-9f85-3fa9c44168e5-amd64 linux/amd64
./build_benzhi_docker.sh benzhi-project-6a699cef-f740-4f44-9f85-3fa9c44168e5-arm64 linux/arm64
docker run -it benzhi-project-6a699cef-f740-4f44-9f85-3fa9c44168e5-amd64:latest

## 题目验证命令
1. 预期退出码 0：`go test ./...`
2. 预期退出码 0：`go run ./cmd/benchslot smoke`
