# LiteCore Windows 新手运行指南

## 你正在运行什么

这个项目是一个用 Go 和 gRPC 写的轻量级 5G 核心网流程仿真。它借用了真实 5G 中 AMF、SMF、UPF 的职责划分，但没有实现 NAS、PFCP、GTP-U，也不会连接真实手机或基站。

四个程序的关系是：

```text
模拟 UE -> AMF -> SMF -> UPF
 手机       门卫    会话管家  转发规则执行者
```

- UE：生成一个虚拟终端和信号参数，发起注册或压测。
- AMF：在UE首次注册时检查参数和信道质量；信号低于 -110 dBm 或 SINR 低于 0 dB 时拒绝接入。注册后不做持续信道监测，也不会因后续信道变差自动下线。
- SMF：创建会话，从 `10.0.x.x` 地址池分配 UE IP，并要求 UPF 创建规则。
- UPF：保存模拟转发规则，周期性增加“已转发包数”；它不转发真实网络包。

## 当前电脑已经准备好的内容

- 源码位于本目录。
- 便携版 Go 1.25.5 位于 `.tools/go`，没有修改系统 PATH。
- Go 依赖缓存位于 `.cache`。
- 全部普通单元测试和 `go vet` 已通过。

## 每次运行只需要三条命令

在本目录打开 PowerShell。

### 1. 启动三个核心网服务

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\start-windows.ps1
```

成功标志：看到 `LiteCore 已启动`，三个端口分别是：

- AMF：50051
- SMF：50052
- UPF：50053

### 2. 跑完整演示

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\demo-windows.ps1
```

脚本依次验证：稳定信道注册、重复注册幂等、查询 UE 状态、注销、恶化信道拒绝、100 UE 并发压测。恶化信道步骤显示一次“注册失败”是正确现象。

也可以单独查询一个 UE：

```powershell
.\.run\bin\ue.exe -action get -id UE-DEMO
```

已注册的 UE 会显示状态、AMF UE ID、会话 ID 和 IP；不存在的 UE 会显示“未找到”。

自定义压测规模：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\demo-windows.ps1 -Count 500 -Concurrency 50 -Scenario mixed
```

CSV 明细保存在 `results`，包含每个请求实际发生的重试次数。延迟汇总只统计成功注册；信道拒绝和其他失败单独计入成功率。benchmark写完结果后会自动注销成功UE并归还IP，控制台应显示清理失败数为0。`mixed` 理论成功率约80.1%，有限样本出现波动是正常现象。

### 3. 停止服务

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\stop-windows.ps1
```

AMF、SMF、UPF都只把状态存在内存中，因此停止后 UE、会话、IP 分配和规则都会清空。不要单独重启一个服务后继续正式实验；需要重启时应重启整个服务栈。

## 怎么看懂一次注册

1. UE 把 `ue_id`、信号功率和 SINR 发给 AMF。
2. AMF 先做信道门控；不合格就直接拒绝。
3. 合格时，AMF 请求 SMF 创建会话。
4. SMF 分配一个 UE IP，再请求 UPF 创建模拟转发规则。
5. UPF 返回规则 ID，SMF 返回会话 ID 和 IP，AMF 最终告诉 UE 注册成功。

任何下游步骤失败，错误会沿链路返回。SMF 在 UPF 创建失败时会回收已经占用的 IP，这叫“失败回滚”。

## 常用排错命令

查看最近日志：

```powershell
Get-Content .\.run\logs\amf.err.log -Tail 30
Get-Content .\.run\logs\smf.err.log -Tail 30
Get-Content .\.run\logs\upf.err.log -Tail 30
```

重新运行普通测试：

```powershell
$env:GOCACHE="$PWD\.cache\go-build"
$env:GOPATH="$PWD\.cache\gopath"
.\.tools\go\bin\go.exe test ./amf/... ./smf/... ./upf/... ./ue/... ./proto/...
```

常见问题：

- “端口已被占用”：先运行 `stop-windows.ps1`；若仍占用，说明有其他程序使用了 50051～50053。
- “找不到便携版 Go”：确认 `.tools/go/bin/go.exe` 存在。
- 下载依赖超时：国内网络可在当前 PowerShell 临时执行 `$env:GOPROXY='https://goproxy.cn,direct'`。
- `go test -race` 提示需要 CGO：Windows 上还需安装 C 编译器；这不影响普通运行和单元测试。

## 读代码的推荐顺序

1. `ue/channel.go`：最短，先理解四种信道场景。
2. `amf/handler.go`：看信道门限、注册和注销入口。
3. `smf/handler.go`：看 IP 分配、会话状态和失败回滚。
4. `upf/handler.go`：看规则如何创建、计数和删除。
5. `proto/*.proto`：看四个程序之间传递的数据结构。

不要把本机测出的毫秒数说成“5G 空口时延”。它测量的是四个 Go 程序之间的软件控制面调用延迟。
