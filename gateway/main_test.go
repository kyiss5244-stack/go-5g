package main

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	amfpb "github.com/5g-core/proto/amf"
	"google.golang.org/grpc"
)

type timingAMFClient struct {
	amfpb.AMFServiceClient
	processingMs float64
}

type capturingAMFClient struct {
	amfpb.AMFServiceClient
	request *amfpb.RegisterRequest
}

func (c *capturingAMFClient) Register(_ context.Context, request *amfpb.RegisterRequest, _ ...grpc.CallOption) (*amfpb.RegisterResponse, error) {
	c.request = request
	return &amfpb.RegisterResponse{Success: true, ProcessingMs: 1}, nil
}

type blockingAMFClient struct{ amfpb.AMFServiceClient }

func (blockingAMFClient) Register(ctx context.Context, _ *amfpb.RegisterRequest, _ ...grpc.CallOption) (*amfpb.RegisterResponse, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (c timingAMFClient) Register(context.Context, *amfpb.RegisterRequest, ...grpc.CallOption) (*amfpb.RegisterResponse, error) {
	return &amfpb.RegisterResponse{Success: true, ProcessingMs: c.processingMs}, nil
}

func TestRegistrationStopsWhenRequestIsCancelled(t *testing.T) {
	g := &gateway{amfClient: blockingAMFClient{}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result := g.registerWithRetry(ctx, "test-request", "UE-CANCEL", channelSample{SignalPower: -80, SINR: 10})
	if result.Success || result.Retries != 0 || result.Message != "注册请求已由 UE 取消" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestRegistrationJSONPreservesProcessingPrecision(t *testing.T) {
	for _, want := range []float64{0.001, 0.35, 0.89, 1.76} {
		g := &gateway{amfClient: timingAMFClient{processingMs: want}}
		result := g.registerWithRetry(context.Background(), "test-request", "UE-PRECISION", channelSample{SignalPower: -80, SINR: 10})
		recorder := httptest.NewRecorder()
		writeJSON(recorder, 200, result)
		var body struct {
			Success      bool    `json:"success"`
			ProcessingMs float64 `json:"processingMs"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if !body.Success || body.ProcessingMs != want {
			t.Errorf("processingMs = %v, want %v; success = %v", body.ProcessingMs, want, body.Success)
		}
	}
}

func TestHandleRegisterUsesVerifiedOAIRecord(t *testing.T) {
	client := &capturingAMFClient{}
	dataset := questDatasetMetadata()
	dataset.Records = []oaiRecord{{Index: 0, SourceLine: 17, RNTI: "46dd", SINRDB: 12.5}}
	g := &gateway{amfClient: client, oaiDataset: dataset, known: make(map[string]time.Time)}
	request := httptest.NewRequest("POST", "/api/register", strings.NewReader(`{"ueId":"OAI-UE-1","oai":{"datasetId":"quest-users2-run1","recordIndex":0,"signalPower":-82.5}}`))
	recorder := httptest.NewRecorder()
	g.handleRegister(recorder, request)
	if recorder.Code != 200 {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if client.request == nil || client.request.SignalPower != -82.5 || client.request.Sinr != 12.5 {
		t.Fatalf("unexpected AMF request: %+v", client.request)
	}
	if !strings.Contains(recorder.Body.String(), `"inputSource":"oai"`) || !strings.Contains(recorder.Body.String(), `"sourceLine":17`) {
		t.Fatalf("missing OAI provenance: %s", recorder.Body.String())
	}
}

func TestHandleRegisterRejectsUnknownOAIRecord(t *testing.T) {
	g := &gateway{oaiDataset: questDatasetMetadata()}
	request := httptest.NewRequest("POST", "/api/register", strings.NewReader(`{"ueId":"OAI-UE-1","oai":{"datasetId":"quest-users2-run1","recordIndex":99,"signalPower":-80}}`))
	recorder := httptest.NewRecorder()
	g.handleRegister(recorder, request)
	if recorder.Code != 400 || !strings.Contains(recorder.Body.String(), "OAI 记录序号无效") {
		t.Fatalf("unexpected response: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
