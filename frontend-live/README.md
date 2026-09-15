# frontend-live · LiteCore 实验台

连接真实后端的网页前端（Vite + React + TypeScript）。这里的每一次注册、注销、在线列表和健康状态都来自运行中的 LiteCore 后端。

## 运行

```bash
npm install
npm run dev   # http://localhost:5173
```

前提：`amf`、`smf`、`upf` 和 `gateway`（HTTP 网关，:8080）已经启动。使用 Docker Compose 可同时启用网元终端的真实日志与启停控制：

```bash
# 在仓库根目录
docker compose up -d --build
```

## 两种模式

- **实时连接**：网关可达时自动进入。页面每 1.5 秒同步在线终端，每 3 秒做一次健康检查。
- **演示模式**：网关连不上时自动降级，顶部出现提示横幅，注册退化为固定信道值的排练数据；页面每 5 秒重试一次网关，后端一起来就自动切回实时。

## 压测结果导出

实时模式完成一轮并发注册压测后，可点击“导出本轮 CSV”。文件包含本轮配置、汇总指标、每个 UE 的信道采样、注册结果、重试次数、处理耗时、资源标识、错误消息和注销清理结果；文件使用 UTF-8 BOM，可直接用 Excel 打开中文内容。

## 接口

全部经由 `gateway`（http://localhost:8080，CORS 允许本机 5173）：

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| POST | `/api/register` | 注册一台 UE，网关侧按 UE 客户端同款策略重试 |
| POST | `/api/deregister` | 注销一台 UE |
| GET | `/api/ues` | 在线终端列表（含 UPF 转发包计数） |
| GET | `/api/health` | 网关 + AMF/SMF/UPF 健康状态 |
| GET | `/api/nodes/{node}/status` | 读取 AMF/SMF/UPF 容器状态 |
| GET | `/api/nodes/{node}/logs` | 读取网元最近 500 条真实容器日志 |
| POST | `/api/nodes/{node}/start` | 启动指定网元容器 |
| POST | `/api/nodes/{node}/stop` | 优雅停止指定网元容器 |

注册路径上的四个节点都可点击打开终端。UE 终端显示浏览器请求与返回，可取消当前注册；其余终端显示对应容器日志，并可停止或重新启动服务。控制器只接受 `amf`、`smf`、`upf` 三个固定服务名，且仅在 Compose 内网访问 Docker socket。

## 构建

```bash
npm run build   # 类型检查 + 产物输出到 dist/
```
