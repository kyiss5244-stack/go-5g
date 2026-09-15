// LiteCore 容器控制器：只允许管理当前 Compose 项目的 AMF、SMF、UPF。
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

var allowedServices = map[string]bool{"amf": true, "smf": true, "upf": true}

type dockerClient struct {
	http    *http.Client
	project string
}

func newDockerClient() *dockerClient {
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", "/var/run/docker.sock")
	}}
	project := os.Getenv("COMPOSE_PROJECT_NAME")
	if project == "" {
		project = "litecore"
	}
	return &dockerClient{http: &http.Client{Transport: transport, Timeout: 12 * time.Second}, project: project}
}

func (d *dockerClient) request(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, "http://docker"+path, body)
	if err != nil {
		return nil, err
	}
	return d.http.Do(req)
}

func (d *dockerClient) containerID(ctx context.Context, service string) (string, error) {
	filters, _ := json.Marshal(map[string][]string{"label": {
		"com.docker.compose.project=" + d.project,
		"com.docker.compose.service=" + service,
	}})
	resp, err := d.request(ctx, http.MethodGet, "/containers/json?all=1&filters="+url.QueryEscape(string(filters)), nil)
	if err != nil {
		return "", fmt.Errorf("连接 Docker 失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", dockerError(resp)
	}
	var containers []struct {
		ID string `json:"Id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&containers); err != nil {
		return "", fmt.Errorf("解析容器列表失败: %w", err)
	}
	if len(containers) == 0 {
		return "", fmt.Errorf("没有找到 %s 容器，请先执行 docker compose up", strings.ToUpper(service))
	}
	return containers[0].ID, nil
}

func dockerError(resp *http.Response) error {
	payload, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var body struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(payload, &body) == nil && body.Message != "" {
		return errors.New(body.Message)
	}
	return fmt.Errorf("Docker 返回 HTTP %d", resp.StatusCode)
}

func (d *dockerClient) status(ctx context.Context, service string) (map[string]any, error) {
	id, err := d.containerID(ctx, service)
	if err != nil {
		return nil, err
	}
	resp, err := d.request(ctx, http.MethodGet, "/containers/"+id+"/json", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, dockerError(resp)
	}
	var body struct {
		Name  string `json:"Name"`
		State struct {
			Status     string `json:"Status"`
			Running    bool   `json:"Running"`
			StartedAt  string `json:"StartedAt"`
			FinishedAt string `json:"FinishedAt"`
		} `json:"State"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	return map[string]any{"service": service, "container": strings.TrimPrefix(body.Name, "/"), "status": body.State.Status, "running": body.State.Running, "startedAt": body.State.StartedAt, "finishedAt": body.State.FinishedAt}, nil
}

func (d *dockerClient) action(ctx context.Context, service, action string) error {
	id, err := d.containerID(ctx, service)
	if err != nil {
		return err
	}
	path := "/containers/" + id + "/" + action
	if action == "stop" {
		path += "?t=5"
	}
	resp, err := d.request(ctx, http.MethodPost, path, bytes.NewReader(nil))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotModified {
		return dockerError(resp)
	}
	return nil
}

func demultiplexLogs(data []byte) []string {
	var lines []string
	for len(data) >= 8 && (data[0] == 1 || data[0] == 2) {
		size := int(binary.BigEndian.Uint32(data[4:8]))
		if size < 0 || len(data) < 8+size {
			break
		}
		appendLogLines(&lines, data[8:8+size])
		data = data[8+size:]
	}
	if len(lines) == 0 && len(data) > 0 {
		appendLogLines(&lines, data)
	}
	return lines
}

func appendLogLines(lines *[]string, data []byte) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			*lines = append(*lines, line)
		}
	}
}

func (d *dockerClient) logs(ctx context.Context, service string) ([]string, error) {
	id, err := d.containerID(ctx, service)
	if err != nil {
		return nil, err
	}
	resp, err := d.request(ctx, http.MethodGet, "/containers/"+id+"/logs?stdout=1&stderr=1&timestamps=1&tail=500", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, dockerError(resp)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	return demultiplexLogs(data), nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func main() {
	docker := newDockerClient()
	mux := http.NewServeMux()
	mux.HandleFunc("/services/", func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) != 3 || parts[0] != "services" || !allowedServices[parts[1]] {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "未知网元"})
			return
		}
		service, action := parts[1], parts[2]
		ctx := r.Context()
		switch {
		case r.Method == http.MethodGet && action == "status":
			value, err := docker.status(ctx, service)
			if err != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, value)
		case r.Method == http.MethodGet && action == "logs":
			lines, err := docker.logs(ctx, service)
			if err != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"service": service, "lines": lines})
		case r.Method == http.MethodPost && (action == "start" || action == "stop"):
			if err := docker.action(ctx, service, action); err != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
				return
			}
			value, err := docker.status(ctx, service)
			if err != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, value)
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "不支持的操作"})
		}
	})
	server := &http.Server{Addr: ":8090", Handler: mux, ReadHeaderTimeout: 3 * time.Second}
	log.Printf("容器控制器启动 project=%s", docker.project)
	log.Fatal(server.ListenAndServe())
}
