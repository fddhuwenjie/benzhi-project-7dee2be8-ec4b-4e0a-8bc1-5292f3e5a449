# 种子活力复核放行工作台

本项目面向种质库样品技术员、质量复核员和试验负责人，将种子老化试验、分轮萌发计数、偏差裁决、质量门禁以及不可变归档组织为一条可追溯工作流。Go 服务同时提供原生浏览器页面和同源 JSON API，数据保存在本地 JSON 快照与追加式审计日志中。

工作台支持按种子批号、作物、负责人和状态组合检索并分页查看流程待办；方案锁定前执行只读影响预检；老化读数按温湿度上下限生成分项偏差；萌发计数强制按轮次和累计规律提交，并在复核前提供带版本历史的受控更正。退回偏差可由原报告技术员整改后复裁，签发门禁校验实际参与人的职责分离。归档页面可以随时重新核验方案、计数、审计链和封存清单摘要，所有证明查询均为只读操作。

## 构建

```bash
go build ./cmd/seedtrial
```

## 运行

```bash
go run ./cmd/seedtrial -addr=127.0.0.1:19081 -data-dir=./data
```

也可以通过 `PORT` 环境变量指定回环监听端口。启动后访问 `http://127.0.0.1:19081/`。服务仅接受回环监听地址，默认不会对外网开放。

## 测试与自检

```bash
go test ./...
go run ./cmd/seedtrial -addr=127.0.0.1:19081 -self-check -self-check-timeout=15s
```

自检会启动真实 HTTP 服务，按顺序执行创建批次、锁定方案、登记条件、提交计数、复核、放行和归档，并在完成后主动退出。

## 主要同源 API

- `GET /api/trials`：使用 `seed_lot_code`、`crop_name`、`owner`、`status`、`offset` 和 `limit` 组合查询，返回 `total` 与待办统计。
- `POST /api/trials/{id}/protocol/preflight`：只读方案预检；确认锁定时将返回的 `summary` 作为 `preflight_summary` 提交到原方案路由。
- `POST /api/trials/{id}/rounds/correction`：更正已有计数并保存不可变版本。
- `POST /api/trials/{id}/deviations/{deviation_id}/remediation`：提交退回偏差的整改版本。
- `GET /api/trials/{id}/gate?signer=...`：只读预览完整性、阈值、偏差终态和职责分离门禁。
- `GET /api/trials/{id}/integrity`：只读查询 ARCHIVED 批次的逐项完整性证明。
- `GET /api/trials/{id}/audit`：使用 `type`、`actor`、`from`、`to`、`offset` 和 `limit` 筛选审计时间线；时间使用 RFC3339。
