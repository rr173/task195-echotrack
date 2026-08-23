# task195-echotrack 声学阵列回波相位追踪台

超声与声呐算法工程师在浏览器中检查阵列采样的相位连续性，标记遮挡区段并复核回波轨迹。

## 业务闭环

1. 注册阵列（阵元编号 + 几何位置 + 硬件延迟），创建采集批次；
2. 上传带阵元编号的复采样窗口（I/Q），服务按 `(batchId, elementNo, seqNo)` 幂等去重；
3. 服务校正阵元延迟、解卷绕相位、拼接相位轨迹并识别跳变；
4. 工程师标注可疑区段、切换参考阵元重算、确认轨迹；
5. 发布冻结的解释包（绑定输入快照）；迟到窗口只能进入新复核版本，旧包不变。

## 核心状态机

| 实体 | 状态机 |
|------|--------|
| 阵列批次 | uploading → processing → reviewing → published → archived |
| 采样窗口 | new → corrected → phase_broken / ignored |
| 回波轨迹 | fitting → continuous / needs_segmentation → confirmed |
| 解释包 | draft → published → superseded |

## 目录结构

```
env/
├── cmd/echotrack/          # 入口（--addr / --db / --smoke-test）
├── internal/
│   ├── model/              # 实体、状态机、统计
│   ├── store/              # SQLite 持久化（建表迁移 + CRUD + 幂等）
│   ├── ingest/             # 采集模块：幂等接收、采样率校验
│   ├── correction/         # 校正模块：相位计算、解卷绕、延迟补偿
│   ├── fitting/            # 拟合模块：跳变检测、自动分段、中位数拼接
│   ├── review/             # 复核模块：标注、分段确认、版本管理
│   ├── service/            # 编排层
│   └── httpapi/            # HTTP 层（27 个 API，前缀 /api）
├── Dockerfile / benzhi.Dockerfile
└── build_benzhi_docker.sh
```

## 标准命令

```bash
export GOTOOLCHAIN=local GOVERSION=1.26.3 CGO_ENABLED=0
export GOPROXY=https://goproxy.cn,direct GOSUMDB=sum.golang.google.cn

go build ./...
go vet   ./...
go test  ./...
go run ./cmd/echotrack --smoke-test    # 端到端自检（含重启恢复验证）

go run ./cmd/echotrack --addr :8080 --db echotrack.db
```

## API 入口（27 个，前缀 /api）

| 能力 | 入口 |
|------|------|
| 注册/查询阵列 | POST /api/arrays · GET /api/arrays · GET /api/arrays/{id} |
| 阵列统计/阵元延迟 | GET /api/arrays/{id}/stats · PUT /api/arrays/{id}/elements/{no}/delay |
| 创建/查询批次 | POST /api/batches · GET /api/batches · GET /api/batches/{id} |
| 上传窗口（幂等） | POST /api/batches/{id}/windows · GET /api/batches/{id}/windows |
| 窗口详情/忽略 | GET /api/batches/{id}/windows/{windowId} · POST .../windows/{el}/{seq}/ignore |
| 处理流水线 | POST /api/batches/{id}/process |
| 切换参考阵元重算 | POST /api/batches/{id}/reference/{elementNo} |
| 轨迹查询 | GET /api/batches/{id}/tracks · GET /api/tracks/{trackId} |
| 人工分段/确认 | POST /api/tracks/{trackId}/segments · POST /api/tracks/{trackId}/confirm |
| 标注 | POST /api/batches/{id}/annotations · GET /api/batches/{id}/annotations |
| 解释包 | POST/GET /api/batches/{id}/interpretations · POST /api/interpretations/{id}/publish · POST /api/interpretations/supersede |
| 系统 | GET /api/stats · GET /api/health |

## 持久化

SQLite（modernc.org/sqlite 纯 Go 驱动，无 CGO）：arrays、elements、batches、windows、
corrections、tracks、segments、annotations、interpretations 九张表。窗口按
`(batch_id, element_no, seq_no)` 唯一键幂等；批次 window_cursor 作为处理游标，
重启后从未完成窗口续传；发布解释包引用固定输入快照（batchId@version）。
