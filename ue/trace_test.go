package main

import (
	"crypto/sha256"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func traceFixture(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "channel.csv")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestImportPreservesDistinctMetricsAndMissingValues(t *testing.T) {
	data := "\ufefftime_ms,ue_id,cell_id,rsrp_dbm,rssi_dbm,sinr_db,snr_db,extra\r\n" +
		"0,UE-A,7,-101.25,-70,,12.5,original\r\n0.5,UE-A,7,-103,-72,-2,,next\r\n"
	path := traceFixture(t, data)
	trace, err := loadRadioTrace(path, "OAI format test fixture; synthetic")
	if err != nil {
		t.Fatal(err)
	}
	if len(trace.Samples) != 2 || trace.SHA256 != fmt.Sprintf("%x", sha256.Sum256([]byte(data))) {
		t.Fatalf("incorrect provenance: %+v", trace)
	}
	first := trace.Samples[0]
	if first.Metrics["rsrp_dbm"] != -101.25 || first.Metrics["rssi_dbm"] != -70 || first.Metrics["snr_db"] != 12.5 {
		t.Fatalf("measurements changed: %+v", first)
	}
	if _, exists := first.Metrics["sinr_db"]; exists {
		t.Fatal("missing SINR was filled")
	}
	if first.Line != 2 || first.UEID != "UE-A" || first.CellID != "7" || trace.Samples[1].TimeMS != 0.5 {
		t.Fatalf("incorrect sample metadata: %+v", trace.Samples)
	}
	if _, err := trace.admissionSamples(); err == nil {
		t.Fatal("RSRP or SNR was silently accepted as gate input")
	}
	output := filepath.Join(t.TempDir(), "imported.csv")
	if err := importChannel(options{input: path, source: "synthetic", output: output}); err != nil {
		t.Fatal(err)
	}
	copy, err := loadRadioTrace(output, "synthetic copy")
	if err != nil || copy.Records[0][7] != "original" || copy.Records[0][5] != "" {
		t.Fatalf("copy lost extra columns or missing values: %v", err)
	}
	if err := importChannel(options{input: path, source: "synthetic", output: path}); err == nil {
		t.Fatal("source overwrite allowed")
	}
	unchanged, err := os.ReadFile(path)
	if err != nil || string(unchanged) != data {
		t.Fatal("source file was changed")
	}
}

func TestTraceRejectsInvalidData(t *testing.T) {
	tests := []struct{ name, csv, want string }{
		{"empty", "", "表头"},
		{"no rows", "time_ms,sinr_db\n", "没有数据行"},
		{"no time", "rsrp_dbm\n-90\n", "time_ms"},
		{"unknown units", "time_ms,RSRP\n0,-90\n", "测量列"},
		{"duplicate", "time_ms,rsrp_dbm,rsrp_dbm\n0,-90,-90\n", "重复表头"},
		{"empty header", "time_ms,,rsrp_dbm\n0,x,-90\n", "表头为空"},
		{"ragged", "time_ms,rsrp_dbm\n0,-90,extra\n", "读取CSV"},
		{"missing", "time_ms,rsrp_dbm\n0,\n", "第2行没有有效测量值"},
		{"nan", "time_ms,rsrp_dbm\n0,NaN\n", "第2行rsrp_dbm"},
		{"infinity", "time_ms,rsrp_dbm\n0,+Inf\n", "有限数值"},
		{"overflow", "time_ms,rsrp_dbm\n0,1e50\n", "有限数值"},
		{"negative time", "time_ms,rsrp_dbm\n-1,-90\n", "非负"},
		{"bad time", "time_ms,rsrp_dbm\nNaN,-90\n", "有限"},
		{"time reversal", "time_ms,rsrp_dbm\n1,-90\n0,-90\n", "第3行time_ms倒序"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseRadioTrace([]byte(test.csv))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("err=%v; want %q", err, test.want)
			}
		})
	}
	if _, err := loadRadioTrace("irrelevant", " "); err == nil {
		t.Fatal("missing provenance accepted")
	}
}

func TestTraceBenchmarkUsesFileSamplesAndExportsProvenance(t *testing.T) {
	data := "time_ms,ue_id,cell_id,signal_power_dbm,sinr_db\n0,source-ue,7,-85.125,15.25\n25.5,source-ue,7,-113.5,-3.125\n"
	path := traceFixture(t, data)
	output := filepath.Join(t.TempDir(), "results.csv")
	client := &fakeAMFClient{}
	err := runBenchmark(client, options{input: path, source: "synthetic test fixture", output: output,
		count: 999, concurrency: 2, timeout: defaultRequestTimeout, runID: "trace-test", scenario: "stable"})
	if err != nil {
		t.Fatal(err)
	}
	if len(client.registerRequests) != 2 || len(client.deregisterRequests) != 2 {
		t.Fatalf("wrong request count: %d/%d", len(client.registerRequests), len(client.deregisterRequests))
	}
	seen := make(map[string]ChannelSample)
	for _, req := range client.registerRequests {
		seen[req.UeId] = ChannelSample{SignalPower: req.SignalPower, SINR: req.Sinr}
	}
	if seen["UE-trace-test-000001"] != (ChannelSample{-85.125, 15.25}) || seen["UE-trace-test-000002"] != (ChannelSample{-113.5, -3.125}) {
		t.Fatalf("file samples or independent UE IDs lost: %v", seen)
	}
	file, err := os.Open(output)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	records, err := csv.NewReader(file).ReadAll()
	if err != nil || len(records) != 3 {
		t.Fatalf("bad output: %v", err)
	}
	columns := make(map[string]int)
	for i, name := range records[0] {
		columns[name] = i
	}
	for name, want := range map[string]string{
		"scenario": "file", "channel_seed": "", "data_source": "synthetic test fixture",
		"source_sha256": fmt.Sprintf("%x", sha256.Sum256([]byte(data))),
		"source_line":   "3", "trace_time_ms": "25.5", "source_ue_id": "source-ue", "source_cell_id": "7",
		"signal_power_dbm": "-113.5", "sinr_db": "-3.125",
	} {
		index, exists := columns[name]
		if !exists || records[2][index] != want {
			t.Errorf("%s: got %q, want %q", name, records[2][index], want)
		}
	}
}

func TestTraceBenchmarkValidatesWholeFileBeforeRPC(t *testing.T) {
	for _, tail := range []string{"1,NaN,3\n", "1,-90,\n"} {
		path := traceFixture(t, "time_ms,signal_power_dbm,sinr_db\n0,-80,20\n"+tail)
		client := &fakeAMFClient{}
		err := runBenchmark(client, options{input: path, source: "synthetic", concurrency: 1, timeout: defaultRequestTimeout})
		if err == nil || len(client.registerRequests) != 0 {
			t.Fatalf("invalid file caused side effects: err=%v calls=%d", err, len(client.registerRequests))
		}
	}
	path := traceFixture(t, "time_ms,signal_power_dbm,sinr_db\n0,-80,20\n")
	client := &fakeAMFClient{}
	if err := runBenchmark(client, options{input: path, output: path, source: "synthetic", concurrency: 1, timeout: defaultRequestTimeout}); err == nil || len(client.registerRequests) != 0 {
		t.Fatal("benchmark can overwrite source or sent requests before checking")
	}
}
