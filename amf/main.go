package main

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	amfpb "github.com/5g-core/proto/amf"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
)

func rpcLogInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	started := time.Now()
	requestID := "-"
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if values := md.Get("x-request-id"); len(values) > 0 {
			requestID = values[0]
		}
		ctx = metadata.NewOutgoingContext(ctx, md.Copy())
	}
	resp, err := handler(ctx, req)
	if strings.Contains(info.FullMethod, "grpc.health") {
		return resp, err
	}
	if err != nil {
		log.Printf("gRPC 返回 request_id=%s method=%s request=%v error=%v duration=%s", requestID, info.FullMethod, req, err, time.Since(started).Round(time.Microsecond))
	} else {
		log.Printf("gRPC 返回 request_id=%s method=%s request=%v response=%v duration=%s", requestID, info.FullMethod, req, resp, time.Since(started).Round(time.Microsecond))
	}
	return resp, err
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func main() {
	listenAddress := env("LISTEN_ADDRESS", ":50051")
	handler, err := NewAMFHandler(env("SMF_ADDRESS", "localhost:50052"), 3*time.Second)
	if err != nil {
		log.Fatalf("初始化 AMF: %v", err)
	}
	defer handler.Close()
	lis, err := net.Listen("tcp", listenAddress)
	if err != nil {
		log.Fatalf("AMF 监听失败: %v", err)
	}
	server := grpc.NewServer(grpc.UnaryInterceptor(rpcLogInterceptor))
	amfpb.RegisterAMFServiceServer(server, handler)
	hs := health.NewServer()
	hs.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(server, hs)
	go func() {
		log.Printf("AMF 服务启动 listen=%s smf=%s", listenAddress, env("SMF_ADDRESS", "localhost:50052"))
		if err := server.Serve(lis); err != nil {
			log.Fatalf("AMF 服务异常: %v", err)
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	hs.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)
	stopGRPCServer(server, 5*time.Second)
}

func stopGRPCServer(server *grpc.Server, timeout time.Duration) {
	done := make(chan struct{})
	go func() {
		server.GracefulStop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		log.Printf("优雅退出超过 %s，强制停止", timeout)
		server.Stop()
		<-done
	}
}
