package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
)

// Metrics retain their measurement semantics. In particular, neither RSRP nor
// SNR is implicitly converted to the admission gate's signal power or SINR.
var traceMetricColumns = []string{"signal_power_dbm", "rsrp_dbm", "rssi_dbm", "sinr_db", "snr_db"}

type radioTraceSample struct {
	TimeMS  float64
	Line    int
	UEID    string
	CellID  string
	Metrics map[string]float32
}

type radioTrace struct {
	Path, Source, SHA256 string
	Samples              []radioTraceSample
	Columns              []string
	Records              [][]string
}

type traceResultMetadata struct {
	Source, SHA256, UEID, CellID string
	TimeMS                       float64
	Line                         int
}

func loadRadioTrace(path, source string) (*radioTrace, error) {
	if path == "" || strings.TrimSpace(source) == "" {
		return nil, fmt.Errorf("必须提供-input和-source（采集软件、版本、RFsim或USRP等来源信息）")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	const maxTraceBytes = 64 * 1024 * 1024
	data, err := io.ReadAll(io.LimitReader(file, maxTraceBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxTraceBytes {
		return nil, fmt.Errorf("信道CSV超过64 MiB，请按实验批次拆分")
	}
	trace, err := parseRadioTrace(data)
	if err != nil {
		return nil, err
	}
	trace.Path, trace.Source = path, strings.TrimSpace(source)
	trace.SHA256 = fmt.Sprintf("%x", sha256.Sum256(data))
	return trace, nil
}

func parseRadioTrace(data []byte) (*radioTrace, error) {
	reader := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})))
	headers, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("读取CSV表头: %w", err)
	}
	columns := make(map[string]int)
	for i, header := range headers {
		header = strings.TrimSpace(header)
		if header == "" {
			return nil, fmt.Errorf("第%d列表头为空", i+1)
		}
		if _, exists := columns[header]; exists {
			return nil, fmt.Errorf("重复表头: %s", header)
		}
		columns[header], headers[i] = i, header
	}
	if _, ok := columns["time_ms"]; !ok {
		return nil, fmt.Errorf("缺少time_ms列（相对采集起点的毫秒数）；OAI原始日志需先按实际格式提取，不能直接当作CSV")
	}
	hasMetric := false
	for _, name := range traceMetricColumns {
		if _, ok := columns[name]; ok {
			hasMetric = true
		}
	}
	if !hasMetric {
		return nil, fmt.Errorf("至少需要一个带单位的测量列: %s；不自动推断单位", strings.Join(traceMetricColumns, ", "))
	}
	trace := &radioTrace{Columns: headers}
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("读取CSV: %w", err)
		}
		line, _ := reader.FieldPos(0)
		value := func(name string) string {
			if index, ok := columns[name]; ok {
				return strings.TrimSpace(record[index])
			}
			return ""
		}
		timeMS, err := parseTraceNumber(value("time_ms"), 64)
		if err != nil || timeMS < 0 {
			return nil, fmt.Errorf("第%d行time_ms必须为有限、非负毫秒数", line)
		}
		if len(trace.Samples) > 0 && timeMS < trace.Samples[len(trace.Samples)-1].TimeMS {
			return nil, fmt.Errorf("第%d行time_ms倒序；请核对时间轴，导入器不会自动排序", line)
		}
		sample := radioTraceSample{TimeMS: timeMS, Line: line, UEID: value("ue_id"), CellID: value("cell_id"), Metrics: make(map[string]float32)}
		for _, name := range traceMetricColumns {
			text := value(name)
			if text == "" {
				continue // A missing measurement must never become zero.
			}
			number, err := parseTraceNumber(text, 32)
			if err != nil {
				return nil, fmt.Errorf("第%d行%s: %w", line, name, err)
			}
			sample.Metrics[name] = float32(number)
		}
		if len(sample.Metrics) == 0 {
			return nil, fmt.Errorf("第%d行没有有效测量值", line)
		}
		trace.Samples = append(trace.Samples, sample)
		trace.Records = append(trace.Records, record)
	}
	if len(trace.Samples) == 0 {
		return nil, fmt.Errorf("CSV没有数据行")
	}
	return trace, nil
}

func parseTraceNumber(value string, bits int) (float64, error) {
	number, err := strconv.ParseFloat(value, bits)
	if err != nil || math.IsNaN(number) || math.IsInf(number, 0) {
		return 0, fmt.Errorf("%q不是有效有限数值", value)
	}
	return number, nil
}

func (t *radioTrace) admissionSamples() ([]ChannelSample, error) {
	samples := make([]ChannelSample, len(t.Samples))
	for i, sample := range t.Samples {
		power, hasPower := sample.Metrics["signal_power_dbm"]
		sinr, hasSINR := sample.Metrics["sinr_db"]
		if !hasPower || !hasSINR {
			return nil, fmt.Errorf("第%d行注册实验需要signal_power_dbm和sinr_db；RSRP/RSSI/SNR仅保留原指标，不能直接改名代替。可先用-action import-channel导入查看", sample.Line)
		}
		samples[i] = ChannelSample{SignalPower: power, SINR: sinr}
	}
	return samples, nil
}

func importChannel(o options) error {
	trace, err := loadRadioTrace(o.input, o.source)
	if err != nil {
		return err
	}
	log.Printf("已校验导入 source=%q file=%q sha256=%s samples=%d time_ms=%g..%g columns=%v",
		trace.Source, trace.Path, trace.SHA256, len(trace.Samples), trace.Samples[0].TimeMS,
		trace.Samples[len(trace.Samples)-1].TimeMS, trace.Columns)
	log.Print("数据来源由用户声明；导入不验证实测真实性，OAI RFsim数据仍属于仿真数据")
	if o.output != "" {
		// Never truncate an existing file, especially the original measurements.
		file, err := os.OpenFile(o.output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			return fmt.Errorf("创建导入副本（不覆盖已有文件）: %w", err)
		}
		writer := csv.NewWriter(file)
		writeErr := writer.WriteAll(append([][]string{trace.Columns}, trace.Records...))
		closeErr := file.Close()
		if writeErr != nil {
			return writeErr
		}
		if closeErr != nil {
			return closeErr
		}
		log.Printf("已保存校验后的CSV副本: %s（原文件未修改；未转换测量值或补齐缺失值）", o.output)
	}
	return nil
}

func protectTraceInput(input, output string) error {
	if output == "" {
		return nil
	}
	in, err := os.Stat(input)
	if err != nil {
		return err
	}
	out, err := os.Stat(output)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if os.SameFile(in, out) {
		return fmt.Errorf("输出文件不能覆盖输入信道文件")
	}
	return nil
}
