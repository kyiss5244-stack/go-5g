package main

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	amfpb "github.com/5g-core/proto/amf"
	"google.golang.org/grpc"
)

type timingAMFClient struct {
	amfpb.AMFServiceClient
	processingMs float64
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
