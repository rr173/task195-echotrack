# BENZHI 评测说明：task195-echotrack

声学阵列回波相位追踪台：多麦阵列复采样窗口的延迟校正、相位连续性分析与回波轨迹拟合。

## 标准命令

```bash
export GOTOOLCHAIN=local GOVERSION=1.26.3 CGO_ENABLED=0
export GOPROXY=https://goproxy.cn,direct GOSUMDB=sum.golang.google.cn

go build ./...
go vet   ./...
go test  ./...
go run ./cmd/echotrack --smoke-test
```

## --smoke-test 契约

`--smoke-test` 不启动长驻服务，而是真实执行：

1. 注册四阵元阵列（48000 Hz）；
2. 创建批次并上传 4 阵元 × 3 窗口（其中阵元 2 注入相位跳变）；
3. 运行处理流水线：延迟校正 + 相位解卷绕 + 轨迹拟合，断言批次进入 reviewing、轨迹生成；
4. 添加标注、人工分段并确认轨迹、切换参考阵元重算；
5. 创建并发布解释包（draft → published）；
6. **关闭并重新打开同一数据库**，验证批次状态、轨迹、标注、解释包全部恢复。

全部断言通过后以退出码 0 结束；任何断言失败以非 0 退出。

## Docker 双架构

```bash
bash build_benzhi_docker.sh task195-echotrack linux/amd64
bash build_benzhi_docker.sh task195-echotrack linux/arm64
docker run --rm task195-echotrack --smoke-test   # 镜像内自检，退出码 0 为通过
```

Dockerfile 以 `golang:1.26.3-bookworm` 构建，`CGO_ENABLED=0`，产物为
`/app/echotrack`，ENTRYPOINT + CMD `["--smoke-test"]`。

## API 摘要

全部 JSON API 前缀 `/api`（27 个）：阵列注册/查询/阵元延迟、批次创建/查询、
窗口幂等上传/忽略、处理流水线、参考阵元切换重算、轨迹查询、人工分段/确认、
区段标注、解释包创建/发布/替代、系统统计与健康检查。详见 README.md。
