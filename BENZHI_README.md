# BENZHI_README

## 项目说明
- 项目：benzhi-project-7dee2be8-ec4b-4e0a-8bc1-5292f3e5a449
- 项目用途：种子活力复核放行工作台已实现批次创建、方案锁定、老化条件快照、分轮萌发计数、偏差裁决、质量门禁、放行签署、不可变归档和只读审计查询，并由 Go 服务提供浏览器页面与同源 JSON API。
- Go 工具链：`golang:1.22`
- 前端工具链：原生 HTML、CSS 和 JavaScript，由 Go 服务直接提供

## 项目描述
- 项目名称：seed-vigor-gate
- 项目介绍：面向种质库技术员的种子活力复核放行工作台，将一批种子的老化试验、计数复核、偏差裁决和不可变归档串成一条可追溯流程。
- 项目概述：面向种质库技术员的种子活力复核放行工作台，将一批种子的老化试验、计数复核、偏差裁决和不可变归档串成一条可追溯流程。
- 核心工作流：创建活力复核批次→锁定试验方案→登记老化条件→提交分轮萌发计数→处理偏差并复核→质量门禁放行→签署封存归档
- 对外接口：Go 服务提供原生 HTML、CSS 和 JavaScript 浏览器工作台，并通过同源 JSON API 完成批次创建、计数提交、复核和归档操作

## 标准构建、运行和测试命令
进入容器后执行：
cd '/app' && GOTOOLCHAIN=local go build ./...

cd /app && GOTOOLCHAIN=local go run ./cmd/seedtrial -addr=127.0.0.1:19081 -self-check -self-check-timeout=15s

cd '/app' && GOTOOLCHAIN=local go test ./...

## Docker 构建和进入容器
chmod +x build_benzhi_docker.sh

./build_benzhi_docker.sh benzhi-project-7dee2be8-ec4b-4e0a-8bc1-5292f3e5a449-amd64 linux/amd64

./build_benzhi_docker.sh benzhi-project-7dee2be8-ec4b-4e0a-8bc1-5292f3e5a449-arm64 linux/arm64

docker run -it benzhi-project-7dee2be8-ec4b-4e0a-8bc1-5292f3e5a449-amd64:latest

## 题目验证命令
1. 预期退出码 0：`go test ./...`
2. 预期退出码 0：`go run ./cmd/seedtrial -addr=127.0.0.1:19081 -self-check -self-check-timeout=15s`
