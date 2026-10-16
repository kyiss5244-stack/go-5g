# OAI 信道数据导入

公开 OAI 数据集的选择、网页接入、历史回放和验收安排见 [公开 OAI 实验数据接入方案](OAI_PUBLIC_DATA_PLAN.md)。本页说明的是现有命令行 CSV 导入能力。

当前已实现命令行 CSV 导入、校验、保存副本，以及用兼容测量列驱动注册实验。**尚未针对某个 OAI 版本的原始日志实现解析器，也没有附带实测数据集。** OAI 的 gNB、nrUE、日志等级和导出工具会影响数据格式，需要根据实际文件对接，不能把任意 `.log` 改名为 `.csv`。

## 先保留哪些原始信息

保留原始日志或导出文件，记录 OAI 版本/commit、gNB 或 nrUE、采集日期、USRP 或 RFsim、频段/带宽及测量方向。OAI RFsim 数据仍是仿真数据。导入器记录文件 SHA-256 以便追溯，但无法验证用户声明的数据是否实测。

如果目前只有服务器终端输出，请先保存完整原始日志，再提供含测量值的连续几行和采集命令。日志里的帧号可能回绕，不能直接当成单调时间；缺少时间或 SINR 时，不应编造这些字段。RSRP、RSSI、SNR、SINR 和原始 I/Q 采样属于不同指标，必须按实际测量定义处理。

## CSV 输入约定

文件使用 UTF-8（支持 BOM），逗号分隔，首行为表头。支持的列如下：

| 列 | 单位/含义 | 要求 |
| --- | --- | --- |
| `time_ms` | 相对采集起点的毫秒数，支持小数 | 必填，有限且非负，全文件非递减；同一时刻多条记录允许 |
| `ue_id` | 采集对象标识，如实际导出的终端标识 | 可选，仅作为来源标识保留 |
| `cell_id` | 原始小区标识 | 可选，具体标识类型在来源说明中注明 |
| `signal_power_dbm` | 已明确测量定义的接收信号功率，dBm | 可选，不能直接用 RSRP/RSSI 改名代替 |
| `rsrp_dbm` | RSRP，dBm | 可选 |
| `rssi_dbm` | RSSI，dBm | 可选 |
| `sinr_db` | SINR，dB | 可选，不接受线性比值冒充 dB |
| `snr_db` | SNR，dB | 可选，不自动转换为 SINR |

至少提供一个受支持的测量列，每行至少有一个测量值；缺失测量留空，不补零。无穷值、NaN、超出 float32 范围的测量值、重复或空表头、列数不一致、时间倒序均会拒绝。为了兼容不同设备，不设未经来源确认的测量范围或自动删掉离群点。单文件最大 64 MiB。

例如只有 RSRP 和 SINR 的文件，表头可以是：

```csv
time_ms,ue_id,cell_id,rsrp_dbm,sinr_db
```

这里不提供虚构的“实测”数据行。实际值来自你的 OAI 原始文件。额外列会在保存副本时保留，但不会被解释为受支持的测量指标。

## 导入并校验

在项目根目录运行（路径替换为实际文件；命令均可作为一行运行）：

```powershell
go run ./ue -action import-channel -input "E:\data\oai-channel.csv" -source "OAI nrUE; commit=<实际版本>; USRP=<型号>; 采集=<日期>; 指标定义=<说明>"
```

此操作不要求后端启动、不发送注册请求，输出行数、时间范围、列名、来源说明及原文件 SHA-256。默认只读校验，不持久保存到服务内存或数据库。

要保存经过校验的 CSV 副本：

```powershell
go run ./ue -action import-channel -input "E:\data\oai-channel.csv" -source "OAI nrUE; <实际采集说明>" -output "E:\data\oai-channel-imported.csv"
```

输出目录需已存在，输出文件必须不存在。原文件不会修改。副本规范化 CSV 编码/表头空白及换行，保留各行字段值、额外列和缺失值，不做排序、插值、单位推断或随机补值。因此副本的文件指纹可能与原文件不同；来源说明和原始 SHA-256 在命令日志中输出，保存实验日志时应一起归档。

## 用文件驱动 LiteCore 注册实验

仅当文件每行都包含定义兼容的 `signal_power_dbm` 和 `sinr_db` 时，才能用于当前 AMF 接入门控实验。只有 RSRP/RSSI/SNR 的文件可以先导入，**不能通过改表头绕过测量定义和门限核实**。后续可以根据研究方案增加适合 RSRP 等指标的独立决策逻辑。

启动 AMF、SMF、UPF 后运行：

```powershell
New-Item -ItemType Directory -Force results
go run ./ue -action benchmark -input "E:\data\oai-channel.csv" -source "OAI nrUE; <实际采集说明>" -concurrency 10 -output results/oai-admission.csv
```

- 使用文件中的全部数据行一次；文件模式忽略 `-count`、`-scenario`、`-seed`，不会循环补齐样本。
- 每行对应一个新的实验 UE，采集对象的 `ue_id` 作为来源字段保留。这样同一实测 UE 的多条记录不会被幂等注册吞掉。
- 按文件行顺序分配任务和导出结果；并发请求的到达/完成顺序不保证与文件顺序一致，需要串行时设 `-concurrency 1`。
- **本版是逐样本的独立首次接入实验，不是按采集时间间隔回放，也不是同一个 UE 的连续移动性实验。** `trace_time_ms` 保留采集时间，实际请求时序仍由压测器决定。
- 结果 CSV 包含 `data_source`、`source_sha256`、`source_line`、`trace_time_ms`、`source_ue_id`、`source_cell_id`；`channel_seed` 留空、`scenario` 为 `file`。测量值经现有 float32 RPC 表达，结果不再强制截成两位小数。
- 整份文件验证通过才会发送请求；输出文件不能与输入文件是同一个文件。实验结束继续使用原有的成功 UE 注销逻辑。

AMF 仍使用教学门限 -110 dBm / 0 dB；结果表示这些输入在 LiteCore 教学规则下的表现，不代表 OAI 的真实接入结果、空口时延或标准规定。UPF 仍为模拟包计数。网页端本轮未增加导入入口。

## 接下来对接实际 OAI 文件

需要一份原始文件或包含测量记录的连续文本，以及采集端（gNB/nrUE）和版本。对接时先确定时间、指标、单位、测量方向与设备标识，再编写对应转换器并用原始样例验证；不能仅凭“OAI”猜测日志格式。
