// LiteCore 前端网关：把后端的 gRPC 能力包装成前端可用的 JSON HTTP API。
//
// 设计原则（对应答辩口径）：
//  1. 网关只"翻译"协议，不碰业务——接入门控、状态机、幂等全部留在 AMF/SMF/UPF；
//  2. 只读加一个"见过谁"的记忆：AMF 没有 List 接口，网关自己记住注册成功的 UE ID，
//     再用 GetUE/GetSession/GetStats 逐个补全实时信息；
//  3. 重试策略与 ue/ 客户端完全一致：仅对 Unavailable/Aborted/DeadlineExceeded
//     重试，最多 2 次，退避 100ms·2^(n-1)，单次超时 4 秒——保证前端看到的
//     重试次数和命令行 UE 是同一套语义。
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"math/rand/v2"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sort"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	amfpb "github.com/5g-core/proto/amf"
	smfpb "github.com/5g-core/proto/smf"
	upfpb "github.com/5g-core/proto/upf"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	defaultRPCTimeout      = 4 * time.Second
	maxRegistrationRetries = 2
	initialRetryWait       = 100 * time.Millisecond
	healthProbeTimeout     = 1500 * time.Millisecond
)

var scenarios = map[string]bool{"stable": true, "edge": true, "degrading": true, "mixed": true}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

// channelSample 与采样分布照搬 ue/channel.go（ScenarioChannelModel.Sample），
// 保证网关与命令行 UE 对同一场景抽出的信道分布完全一致。
type channelSample struct{ SignalPower, SINR float32 }

func sampleChannel(scenario string, rng *rand.Rand) channelSample {
	switch scenario {
	case "stable":
		return channelSample{SignalPower: float32(-82 + rng.NormFloat64()*2), SINR: float32(18 + rng.NormFloat64()*1.5)}
	case "edge":
		return channelSample{SignalPower: float32(-109 + rng.NormFloat64()*4), SINR: float32(2 + rng.NormFloat64()*2)}
	case "degrading":
		return channelSample{SignalPower: float32(-116 + rng.NormFloat64()*3), SINR: float32(-1 + rng.NormFloat64()*2)}
	default: // mixed：70% 稳定、20% 边缘、10% 恶化
		value := rng.Float64()
		if value < .7 {
			return sampleChannel("stable", rng)
		}
		if value < .9 {
			return sampleChannel("edge", rng)
		}
		return sampleChannel("degrading", rng)
	}
}

func round2(value float32) float32 { return float32(math.Round(float64(value)*100)) / 100 }

type gateway struct {
	mu             sync.RWMutex
	known          map[string]time.Time // key:ue_id → 注册成功时间。AMF 无 List 接口，网关自记"见过谁"
	amfClient      amfpb.AMFServiceClient
	smfClient      smfpb.SMFServiceClient
	upfClient      upfpb.UPFServiceClient
	amfHealth      healthpb.HealthClient
	smfHealth      healthpb.HealthClient
	upfHealth      healthpb.HealthClient
	amfAddress     string
	smfAddress     string
	upfAddress     string
	controlAddress string
	controlClient  *http.Client
	nextRequest    atomic.Uint64
	rng            *rand.Rand
	oaiDataset     *oaiDataset
}

func newGateway() *gateway {
	amfAddress := env("AMF_ADDRESS", "localhost:50051")
	smfAddress := env("SMF_ADDRESS", "localhost:50052")
	upfAddress := env("UPF_ADDRESS", "localhost:50053")
	dial := func(address string) *grpc.ClientConn {
		conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			// 参数错误才会走到这里；服务没起不影响——gRPC 惰性连接，请求时才报错
			log.Fatalf("创建 gRPC 客户端失败 address=%s: %v", address, err)
		}
		return conn
	}
	return &gateway{
		known:          make(map[string]time.Time),
		amfClient:      amfpb.NewAMFServiceClient(dial(amfAddress)),
		smfClient:      smfpb.NewSMFServiceClient(dial(smfAddress)),
		upfClient:      upfpb.NewUPFServiceClient(dial(upfAddress)),
		amfHealth:      healthpb.NewHealthClient(dial(amfAddress)),
		smfHealth:      healthpb.NewHealthClient(dial(smfAddress)),
		upfHealth:      healthpb.NewHealthClient(dial(upfAddress)),
		amfAddress:     amfAddress,
		smfAddress:     smfAddress,
		upfAddress:     upfAddress,
		controlAddress: env("CONTROL_ADDRESS", "http://localhost:8090"),
		controlClient:  &http.Client{Timeout: 15 * time.Second},
		rng:            rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), uint64(os.Getpid()))),
		oaiDataset:     loadQuestDataset(defaultOAIDataPath()),
	}
}

// ---------- 与 ue/ 一致的注册重试 ----------

// isRetryableRegistrationError 与 ue/main.go 保持一致。
func isRetryableRegistrationError(err error) bool {
	switch status.Code(err) {
	case codes.Aborted, codes.DeadlineExceeded, codes.Unavailable:
		return true
	default:
		return false
	}
}

func retryWait(retryNumber int) time.Duration {
	return initialRetryWait * time.Duration(1<<(retryNumber-1))
}

type registerResult struct {
	Success      bool    `json:"success"`
	UEID         string  `json:"ueId"`
	AMFUEID      string  `json:"amfUeId,omitempty"`
	SessionID    string  `json:"sessionId,omitempty"`
	UEIP         string  `json:"ueIp,omitempty"`
	SignalPower  float32 `json:"signalPower"`
	SINR         float32 `json:"sinr"`
	Retries      int     `json:"retries"`
	ProcessingMs float64 `json:"processingMs"`
	GatewayMs    float64 `json:"gatewayMs"`
	Message      string  `json:"message"`
	InputSource  string  `json:"inputSource,omitempty"`
	DatasetID    string  `json:"datasetId,omitempty"`
	RecordIndex  *int    `json:"recordIndex,omitempty"`
	SourceLine   int     `json:"sourceLine,omitempty"`
	RNTI         string  `json:"rnti,omitempty"`
}

func (g *gateway) registerWithRetry(parent context.Context, requestID, ueID string, sample channelSample) registerResult {
	started := time.Now()
	parent = metadata.AppendToOutgoingContext(parent, "x-request-id", requestID)
	result := registerResult{UEID: ueID, SignalPower: round2(sample.SignalPower), SINR: round2(sample.SINR)}
	request := &amfpb.RegisterRequest{UeId: ueID, UeType: "web", SignalPower: sample.SignalPower, Sinr: sample.SINR}
	for attempt := 0; attempt <= maxRegistrationRetries; attempt++ {
		ctx, cancel := context.WithTimeout(parent, defaultRPCTimeout)
		resp, err := g.amfClient.Register(ctx, request)
		cancel()
		if err == nil {
			if resp == nil {
				result.Message = "AMF 返回空注册响应"
			} else {
				result.Success = resp.Success
				result.Message = resp.Message
				result.AMFUEID = resp.AmfUeId
				result.SessionID = resp.SessionId
				result.UEIP = resp.UeIp
				result.ProcessingMs = resp.ProcessingMs
			}
			result.GatewayMs = float64(time.Since(started)) / float64(time.Millisecond)
			return result
		}
		result.Message = err.Error()
		if parent.Err() != nil {
			result.Message = "注册请求已由 UE 取消"
			break
		}
		if attempt == maxRegistrationRetries || !isRetryableRegistrationError(err) {
			break
		}
		result.Retries++
		timer := time.NewTimer(retryWait(result.Retries))
		select {
		case <-parent.Done():
			timer.Stop()
			result.Message = "注册请求已由 UE 取消"
			result.GatewayMs = float64(time.Since(started)) / float64(time.Millisecond)
			return result
		case <-timer.C:
		}
	}
	result.GatewayMs = float64(time.Since(started)) / float64(time.Millisecond)
	return result
}

// ---------- HTTP 处理 ----------

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("写响应失败: %v", err)
	}
}

func writeError(w http.ResponseWriter, statusCode int, message string) {
	writeJSON(w, statusCode, map[string]string{"error": message})
}

func (g *gateway) handleRegister(w http.ResponseWriter, r *http.Request) {
	var body struct {
		UEID     string `json:"ueId"`
		Scenario string `json:"scenario"`
		OAI      *struct {
			DatasetID   string   `json:"datasetId"`
			RecordIndex *int     `json:"recordIndex"`
			SignalPower *float64 `json:"signalPower"`
		} `json:"oai,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "请求体不是合法 JSON")
		return
	}
	body.UEID = trimUEID(body.UEID)
	if body.UEID == "" || len(body.UEID) > 48 {
		writeError(w, http.StatusBadRequest, "请填写 1–48 个字符的终端标识")
		return
	}
	var sample channelSample
	var oaiRecord *oaiRecord
	if body.OAI != nil {
		if body.OAI.DatasetID != questDatasetID || g.oaiDataset == nil || g.oaiDataset.LoadError != "" {
			writeError(w, http.StatusBadRequest, "请选择可用的 OAI 数据集")
			return
		}
		if body.OAI.RecordIndex == nil || *body.OAI.RecordIndex < 0 || *body.OAI.RecordIndex >= len(g.oaiDataset.Records) {
			writeError(w, http.StatusBadRequest, "OAI 记录序号无效")
			return
		}
		if body.OAI.SignalPower == nil || math.IsNaN(*body.OAI.SignalPower) || math.IsInf(*body.OAI.SignalPower, 0) || *body.OAI.SignalPower < -200 || *body.OAI.SignalPower > 0 {
			writeError(w, http.StatusBadRequest, "实验设定信号功率必须在 -200 至 0 dBm 之间")
			return
		}
		record := &g.oaiDataset.Records[*body.OAI.RecordIndex]
		oaiRecord = record
		sample = channelSample{SignalPower: float32(*body.OAI.SignalPower), SINR: float32(record.SINRDB)}
	} else {
		if !scenarios[body.Scenario] {
			writeError(w, http.StatusBadRequest, "请选择有效的信道场景")
			return
		}
		sample = sampleChannel(body.Scenario, g.rng)
	}
	requestID := fmt.Sprintf("reg-%d-%d", time.Now().UnixMilli(), g.nextRequest.Add(1))
	w.Header().Set("X-Request-ID", requestID)
	result := g.registerWithRetry(r.Context(), requestID, body.UEID, sample)
	if oaiRecord != nil {
		result.InputSource = "oai"
		result.DatasetID = body.OAI.DatasetID
		result.RecordIndex = &oaiRecord.Index
		result.SourceLine = oaiRecord.SourceLine
		result.RNTI = oaiRecord.RNTI
	}
	if result.Success {
		g.mu.Lock()
		g.known[body.UEID] = time.Now()
		g.mu.Unlock()
		log.Printf("注册成功 request_id=%s ue_id=%s source=%s source_line=%d amf_ue_id=%s session_id=%s ue_ip=%s", requestID, body.UEID, result.InputSource, result.SourceLine, result.AMFUEID, result.SessionID, result.UEIP)
	} else {
		log.Printf("注册失败 request_id=%s ue_id=%s retries=%d message=%s", requestID, body.UEID, result.Retries, result.Message)
	}
	writeJSON(w, http.StatusOK, result)
}

// trimUEID 去掉两端空白，并统一把内部空白换成连字符，避免演示时手滑输入带空格。
func trimUEID(ueID string) string {
	var builder []byte
	for i := 0; i < len(ueID); i++ {
		switch {
		case ueID[i] == ' ' || ueID[i] == '\t':
			builder = append(builder, '-')
		default:
			builder = append(builder, ueID[i])
		}
	}
	return string(builder)
}

func (g *gateway) handleDeregister(w http.ResponseWriter, r *http.Request) {
	var body struct {
		UEID string `json:"ueId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "请求体不是合法 JSON")
		return
	}
	body.UEID = trimUEID(body.UEID)
	if body.UEID == "" || len(body.UEID) > 48 {
		writeError(w, http.StatusBadRequest, "请填写 1–48 个字符的终端标识")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultRPCTimeout)
	current, err := g.amfClient.GetUE(ctx, &amfpb.GetUERequest{UeId: body.UEID})
	cancel()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, fmt.Sprintf("查询 AMF 失败: %v", err))
		return
	}
	if current == nil || !current.Found {
		g.forgetUE(body.UEID)
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "ueId": body.UEID, "message": "UE 已不存在，无需重复注销"})
		return
	}
	releasedIP := current.UeIp

	ctx, cancel = context.WithTimeout(context.Background(), defaultRPCTimeout)
	resp, err := g.amfClient.Deregister(ctx, &amfpb.DeregisterRequest{UeId: body.UEID, AmfUeId: current.AmfUeId})
	cancel()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, fmt.Sprintf("注销失败: %v", err))
		return
	}
	if resp == nil {
		writeError(w, http.StatusServiceUnavailable, "AMF 返回空注销响应")
		return
	}
	if resp.Success {
		g.forgetUE(body.UEID)
		log.Printf("注销成功 ue_id=%s released_ip=%s", body.UEID, releasedIP)
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": resp.Success, "ueId": body.UEID, "message": resp.Message, "releasedIp": releasedIP})
}

func (g *gateway) forgetUE(ueID string) {
	g.mu.Lock()
	delete(g.known, ueID)
	g.mu.Unlock()
}

type ueEntry struct {
	UEID             string `json:"ueId"`
	AMFUEID          string `json:"amfUeId,omitempty"`
	SessionID        string `json:"sessionId,omitempty"`
	UEIP             string `json:"ueIp,omitempty"`
	AMFState         string `json:"amfState,omitempty"`
	SessionState     string `json:"sessionState,omitempty"`
	RuleID           string `json:"ruleId,omitempty"`
	PacketsForwarded uint64 `json:"packetsForwarded"`
	Active           bool   `json:"active"`
	RegisteredAt     string `json:"registeredAt,omitempty"`
}

func (g *gateway) handleListUEs(w http.ResponseWriter, _ *http.Request) {
	g.mu.RLock()
	ids := make([]string, 0, len(g.known))
	registeredAt := make(map[string]time.Time, len(g.known))
	for id, at := range g.known {
		ids = append(ids, id)
		registeredAt[id] = at
	}
	g.mu.RUnlock()
	sort.Strings(ids)

	entries := make([]ueEntry, 0, len(ids))
	for _, id := range ids {
		ctx, cancel := context.WithTimeout(context.Background(), defaultRPCTimeout)
		ue, err := g.amfClient.GetUE(ctx, &amfpb.GetUERequest{UeId: id})
		cancel()
		if err != nil || ue == nil || !ue.Found {
			// 后端已经把这条记录清掉了，网关的记忆也该跟着更新
			if err == nil {
				g.forgetUE(id)
			}
			continue
		}
		entry := ueEntry{UEID: ue.UeId, AMFUEID: ue.AmfUeId, SessionID: ue.SessionId, UEIP: ue.UeIp, AMFState: ue.State, RegisteredAt: registeredAt[id].Format(time.RFC3339)}
		if ue.SessionId != "" {
			ctx, cancel = context.WithTimeout(context.Background(), defaultRPCTimeout)
			session, err := g.smfClient.GetSession(ctx, &smfpb.GetSessionRequest{SessionId: ue.SessionId})
			cancel()
			if err == nil && session != nil && session.Found {
				entry.SessionState = session.State
				entry.RuleID = session.RuleId
				if session.UeIp != "" {
					entry.UEIP = session.UeIp
				}
				ctx, cancel = context.WithTimeout(context.Background(), defaultRPCTimeout)
				stats, err := g.upfClient.GetStats(ctx, &upfpb.GetStatsRequest{SessionId: ue.SessionId})
				cancel()
				if err == nil && stats != nil && stats.Found {
					entry.PacketsForwarded = stats.PacketsForwarded
					entry.Active = stats.Active
				}
			}
		}
		entries = append(entries, entry)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ues": entries})
}

type serviceHealth struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	Status  string `json:"status"` // SERVING / NOT_SERVING / UNREACHABLE
}

func probeHealth(client healthpb.HealthClient, address string) string {
	ctx, cancel := context.WithTimeout(context.Background(), healthProbeTimeout)
	resp, err := client.Check(ctx, &healthpb.HealthCheckRequest{})
	cancel()
	if err != nil {
		return "UNREACHABLE"
	}
	switch resp.Status {
	case healthpb.HealthCheckResponse_SERVING:
		return "SERVING"
	default:
		return "NOT_SERVING"
	}
}

func (g *gateway) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"gateway":   "up",
		"checkedAt": time.Now().Format(time.RFC3339),
		"services": []serviceHealth{
			{Name: "AMF", Address: g.amfAddress, Status: probeHealth(g.amfHealth, g.amfAddress)},
			{Name: "SMF", Address: g.smfAddress, Status: probeHealth(g.smfHealth, g.smfAddress)},
			{Name: "UPF", Address: g.upfAddress, Status: probeHealth(g.upfHealth, g.upfAddress)},
		},
	})
}

var controllableNodes = map[string]bool{"amf": true, "smf": true, "upf": true}

// handleNodeControl 把固定的网元操作转交给 Compose 内网控制器。浏览器不能传入
// 容器名、Docker 路径或任意命令，网关只接受三种已知网元与四种已知动作。
func (g *gateway) handleNodeControl(w http.ResponseWriter, r *http.Request) {
	node := r.PathValue("node")
	action := r.PathValue("action")
	if !controllableNodes[node] {
		writeError(w, http.StatusNotFound, "未知网元")
		return
	}
	if (r.Method == http.MethodGet && action != "status" && action != "logs") ||
		(r.Method == http.MethodPost && action != "start" && action != "stop") {
		writeError(w, http.StatusMethodNotAllowed, "不支持的网元操作")
		return
	}
	target := g.controlAddress + "/services/" + node + "/" + action
	req, err := http.NewRequestWithContext(r.Context(), r.Method, target, nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法创建控制请求")
		return
	}
	resp, err := g.controlClient.Do(req)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "容器控制器不可用，请检查 Docker Compose")
		return
	}
	defer resp.Body.Close()
	var payload any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadGateway, "容器控制器返回了无效响应")
		return
	}
	writeJSON(w, resp.StatusCode, payload)
}

// ---------- 中间件 ----------

// isLocalOrigin 只放行本机网页（localhost/127.0.0.1，任意端口），
// 避免 Vite 换端口（如 5174）时前端被浏览器拦截成"演示模式"。
func isLocalOrigin(origin string) bool {
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	host := parsed.Hostname()
	return host == "localhost" || host == "127.0.0.1"
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if isLocalOrigin(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func requestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(started).Round(time.Millisecond))
	})
}

func main() {
	g := newGateway()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", g.handleHealth)
	mux.HandleFunc("GET /api/ues", g.handleListUEs)
	mux.HandleFunc("POST /api/register", g.handleRegister)
	mux.HandleFunc("POST /api/deregister", g.handleDeregister)
	mux.HandleFunc("GET /api/oai/datasets", g.handleOAIDatasets)
	mux.HandleFunc("GET /api/oai/datasets/{id}", g.handleOAIDataset)
	mux.HandleFunc("GET /api/oai/datasets/{id}/records", g.handleOAIRecords)
	mux.HandleFunc("GET /api/oai/datasets/{id}/series", g.handleOAISeries)
	mux.HandleFunc("GET /api/oai/datasets/{id}/export", g.handleOAIExport)
	mux.HandleFunc("GET /api/nodes/{node}/{action}", g.handleNodeControl)
	mux.HandleFunc("POST /api/nodes/{node}/{action}", g.handleNodeControl)

	listenAddress := env("GATEWAY_ADDRESS", ":8080")
	server := &http.Server{Addr: listenAddress, Handler: requestLog(cors(mux))}
	go func() {
		log.Printf("网关启动 listen=%s AMF=%s SMF=%s UPF=%s", listenAddress, g.amfAddress, g.smfAddress, g.upfAddress)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("网关异常退出: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Printf("网关收到退出信号，开始优雅停止")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("优雅停止超时: %v", err)
	}
}
