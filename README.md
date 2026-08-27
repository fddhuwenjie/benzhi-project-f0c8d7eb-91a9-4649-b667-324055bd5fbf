# 射电观测数据发布资格工作台

本项目面向射电天文台数据值守员与科学复核员，用一条可追溯流程管理观测批次的发布资格。系统支持冻结版本化观测基线、登记数据段、按固定阈值执行确定性射频干扰与完整性质检、隔离失败段并登记补观替换、生成可复现的独立抽审清单，以及封存带 SHA-256 摘要的不可变批准或拒绝清单。

服务由 Go 直接提供浏览器工作台和同源 JSON API，不需要 Node 构建链。所有写操作都要求 `request_id`、`expected_revision` 和 `actor`；重复请求可以幂等重放，陈旧修订会被拒绝，批准或拒绝终态只允许读取。

## 构建

```bash
go build ./cmd/server
```

## 运行

默认监听高位回环地址 `127.0.0.1:19081`，数据保存在当前目录的 `data`：

```bash
go run ./cmd/server
```

可通过 `-addr` 指定完整地址或纯端口，通过 `-data-dir` 指定持久化目录：

```bash
go run ./cmd/server -addr=127.0.0.1:19181 -data-dir=./runtime-data
```

未显式传入 `-addr` 时，也可设置 `PORT`。`PORT` 只接收端口号，服务将绑定 `127.0.0.1:<PORT>`，不会默认绑定 `0.0.0.0`。

浏览器访问 `http://127.0.0.1:19081/`。工作台包含批次目录组合筛选与待办统计、冻结基线、单段及批量登记、单段及整批质检、补观覆盖预演、问题队列、抽审任务分派与进度、审计时间线和清单摘要复算视图。

## 测试

运行全部单元测试和 HTTP 回归测试：

```bash
go test ./...
```

运行有界真实 HTTP 自检。该命令使用临时数据目录，在指定回环地址上启动服务，通过相同 API 完成批准链路，验证发布清单摘要后主动关闭：

```bash
go run ./cmd/server -selftest -addr=127.0.0.1:19091
```

## 持久化

每个批次保存在 `data/batches/<batch_id>/`：

- `snapshot.json`：原子替换的聚合快照。
- `events.log`：4 字节长度定界的追加事件帧，包含连续序号和前序 SHA-256 摘要链。
- `idempotency.json`：`request_id`、请求指纹和响应的持久幂等索引。
- `manifest.json`：以排他创建方式写入的不可变终态清单。

根目录的 `data/content-index.json` 记录跨批次内容摘要归属。服务启动和健康检查会验证快照修订、事件链、事件语义、摘要索引和终态清单的一致性。

## 主要 API

- `GET /`：浏览器工作台。
- `GET /healthz`：持久化完整性健康检查。
- `GET|POST /api/batches`：分页筛选或创建批次。查询支持 `batch_id`、`telescope_id`、`target_source`、`state`、`todo`、`page` 和 `page_size`。
- `GET /api/batches/{batchID}`：读取批次详情、覆盖报告和资格阻断项。
- `POST /api/batches/{batchID}/freeze`：冻结基线。
- `POST /api/batches/{batchID}/segments`：登记原始或替换数据段。
- `POST /api/batches/{batchID}/segments/bulk`：原子批量登记数据段。
- `POST /api/batches/{batchID}/quality`：执行确定性质检。
- `POST /api/batches/{batchID}/quality/bulk`：对全部待检数据段执行整批确定性质检。
- `POST /api/batches/{batchID}/replacement-preview`：只读预演补观替换的质检与覆盖结果。
- `POST /api/batches/{batchID}/quarantine`：确认隔离原因和补观计划。
- `POST /api/batches/{batchID}/reviews`：生成并锁定抽审清单。
- `POST /api/batches/{batchID}/review-decisions`：提交独立复核结论。
- `POST /api/batches/{batchID}/review-assignments`：分派或重新分派待审项目及完成期限。
- `POST /api/batches/{batchID}/seal`：封存批准或拒绝终态。
- `GET /api/batches/{batchID}/timeline`：读取审计时间线与完整性报告。
- `GET /api/batches/{batchID}/manifest/verify`：重新计算并核对清单摘要。
