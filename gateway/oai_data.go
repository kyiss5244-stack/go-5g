package main

import (
	"bufio"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	questDatasetID = "quest-users2-run1"
	questSourceURL = "https://github.com/tum-lkn/QUEST/blob/58e64a649b6e47dbeff10db01ce16c7df0d5e75f/datasets/users_2/run_1/scheduler_QUEST/gNB_results.csv"
	questSourceSHA = "62d57a8682b45fb918bec804175fbe438fcf5c5f6685a4276796438549fbbacb"
)

type oaiRecord struct {
	Index               int     `json:"index"`
	SourceLine          int     `json:"sourceLine"`
	Frame               int     `json:"frame"`
	Slot                int     `json:"slot"`
	RNTI                string  `json:"rnti"`
	SINRDB              float64 `json:"sinrDb"`
	TransportBlockBytes int     `json:"transportBlockBytes"`
	MCS                 int     `json:"mcs"`
	PRBs                int     `json:"prbs"`
	PendingULBytes      int     `json:"pendingUlBytes"`
	HOLDelayMS          float64 `json:"holDelayMs"`
	ULDelayMS           float64 `json:"ulDelayMs"`
}

type oaiDataset struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Source      string      `json:"source"`
	SourceURL   string      `json:"sourceUrl"`
	SourceSHA   string      `json:"sourceSha256"`
	Commit      string      `json:"commit"`
	Environment string      `json:"environment"`
	License     string      `json:"license"`
	TimeAxis    string      `json:"timeAxis"`
	Records     []oaiRecord `json:"-"`
	UEs         []string    `json:"ues"`
	MinSINR     float64     `json:"minSinrDb"`
	MaxSINR     float64     `json:"maxSinrDb"`
	LoadError   string      `json:"loadError,omitempty"`
}

type oaiDatasetSummary struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Source      string   `json:"source"`
	SourceURL   string   `json:"sourceUrl"`
	SourceSHA   string   `json:"sourceSha256"`
	Commit      string   `json:"commit"`
	Environment string   `json:"environment"`
	License     string   `json:"license"`
	TimeAxis    string   `json:"timeAxis"`
	RecordCount int      `json:"recordCount"`
	UEs         []string `json:"ues"`
	MinSINR     float64  `json:"minSinrDb"`
	MaxSINR     float64  `json:"maxSinrDb"`
	LoadError   string   `json:"loadError,omitempty"`
}

type oaiSeriesStats struct {
	Count     int     `json:"count"`
	AvgSINRDB float64 `json:"avgSinrDb"`
	AvgMCS    float64 `json:"avgMcs"`
	AvgPRBs   float64 `json:"avgPrbs"`
}

func (d *oaiDataset) summary() oaiDatasetSummary {
	return oaiDatasetSummary{
		ID: d.ID, Name: d.Name, Source: d.Source, SourceURL: d.SourceURL, SourceSHA: d.SourceSHA,
		Commit: d.Commit, Environment: d.Environment, License: d.License, TimeAxis: d.TimeAxis,
		RecordCount: len(d.Records), UEs: d.UEs, MinSINR: d.MinSINR, MaxSINR: d.MaxSINR, LoadError: d.LoadError,
	}
}

func questDatasetMetadata() *oaiDataset {
	return &oaiDataset{
		ID: questDatasetID, Name: "QUEST 双终端上行调度实验", Source: "TUM-LKN / QUEST",
		SourceURL: questSourceURL, SourceSHA: questSourceSHA, Commit: "58e64a649b6e47dbeff10db01ce16c7df0d5e75f",
		Environment: "公开 OAI 实验记录；仓库说明未明确该文件是否经过 USRP 空口，平台不将其标记为空口实测",
		License:     "Apache-2.0（QUEST 仓库）", TimeAxis: "原始 5G NR 帧/时隙；本版按记录序号回放",
	}
}

func defaultOAIDataPath() string {
	if configured := strings.TrimSpace(os.Getenv("OAI_DATA_FILE")); configured != "" {
		return configured
	}
	for _, candidate := range []string{
		filepath.Join("datasets", "oai", "quest", "gNB_results.csv"),
		filepath.Join("..", "datasets", "oai", "quest", "gNB_results.csv"),
		filepath.Join(string(filepath.Separator), "usr", "share", "litecore", "oai", "gNB_results.csv"),
	} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return filepath.Join("datasets", "oai", "quest", "gNB_results.csv")
}

func loadQuestDataset(path string) *oaiDataset {
	dataset := questDatasetMetadata()
	file, err := os.Open(path)
	if err != nil {
		dataset.LoadError = fmt.Sprintf("公开 OAI 数据文件不可用: %v", err)
		return dataset
	}
	defer file.Close()
	records, err := parseQuestCSV(file)
	if err != nil {
		dataset.LoadError = fmt.Sprintf("公开 OAI 数据文件解析失败: %v", err)
		return dataset
	}
	dataset.Records = records
	dataset.MinSINR, dataset.MaxSINR = math.Inf(1), math.Inf(-1)
	ueSet := make(map[string]struct{})
	for _, record := range records {
		ueSet[record.RNTI] = struct{}{}
		dataset.MinSINR = math.Min(dataset.MinSINR, record.SINRDB)
		dataset.MaxSINR = math.Max(dataset.MaxSINR, record.SINRDB)
	}
	for rnti := range ueSet {
		dataset.UEs = append(dataset.UEs, rnti)
	}
	sort.Strings(dataset.UEs)
	return dataset
}

func parseQuestCSV(input io.Reader) ([]oaiRecord, error) {
	reader := csv.NewReader(bufio.NewReader(input))
	reader.TrimLeadingSpace = true
	reader.FieldsPerRecord = 10
	var records []oaiRecord
	for {
		row, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		line, _ := reader.FieldPos(0)
		parseInt := func(column int, name string) (int, error) {
			value, err := strconv.Atoi(strings.TrimSpace(row[column]))
			if err != nil {
				return 0, fmt.Errorf("第%d行%s无效: %w", line, name, err)
			}
			return value, nil
		}
		parseFloat := func(column int, name string) (float64, error) {
			value, err := strconv.ParseFloat(strings.TrimSpace(row[column]), 64)
			if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
				return 0, fmt.Errorf("第%d行%s无效", line, name)
			}
			return value, nil
		}
		frame, err := parseInt(0, "frame")
		if err != nil {
			return nil, err
		}
		slot, err := parseInt(1, "slot")
		if err != nil {
			return nil, err
		}
		sinr, err := parseFloat(3, "SINR")
		if err != nil {
			return nil, err
		}
		tbs, err := parseInt(4, "transport block")
		if err != nil {
			return nil, err
		}
		mcs, err := parseInt(5, "MCS")
		if err != nil {
			return nil, err
		}
		prbs, err := parseInt(6, "PRB")
		if err != nil {
			return nil, err
		}
		pending, err := parseInt(7, "pending UL bytes")
		if err != nil {
			return nil, err
		}
		hol, err := parseFloat(8, "HOL delay")
		if err != nil {
			return nil, err
		}
		ulDelay, err := parseFloat(9, "UL delay")
		if err != nil {
			return nil, err
		}
		if frame < 0 || slot < 0 || tbs < 0 || mcs < 0 || prbs < 0 || pending < 0 || hol < 0 || ulDelay < 0 {
			return nil, fmt.Errorf("第%d行包含负的计数或时延", line)
		}
		rnti := strings.ToLower(strings.TrimSpace(row[2]))
		if rnti == "" {
			return nil, fmt.Errorf("第%d行RNTI为空", line)
		}
		records = append(records, oaiRecord{Index: len(records), SourceLine: line, Frame: frame, Slot: slot, RNTI: rnti,
			SINRDB: sinr, TransportBlockBytes: tbs, MCS: mcs, PRBs: prbs, PendingULBytes: pending, HOLDelayMS: hol, ULDelayMS: ulDelay})
	}
	if len(records) == 0 {
		return nil, errors.New("没有数据行")
	}
	return records, nil
}

func (g *gateway) handleOAIDatasets(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"datasets": []oaiDatasetSummary{g.oaiDataset.summary()}})
}

func (g *gateway) handleOAIDataset(w http.ResponseWriter, r *http.Request) {
	if r.PathValue("id") != questDatasetID {
		writeError(w, http.StatusNotFound, "OAI 数据集不存在")
		return
	}
	writeJSON(w, http.StatusOK, g.oaiDataset.summary())
}

func boundedQueryInt(r *http.Request, name string, fallback, maximum int) (int, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("%s必须为非负整数", name)
	}
	if value > maximum {
		value = maximum
	}
	return value, nil
}

func (g *gateway) filteredOAIRecords(rnti string) []oaiRecord {
	if rnti == "" {
		return g.oaiDataset.Records
	}
	rnti = strings.ToLower(rnti)
	filtered := make([]oaiRecord, 0)
	for _, record := range g.oaiDataset.Records {
		if record.RNTI == rnti {
			filtered = append(filtered, record)
		}
	}
	return filtered
}

func (g *gateway) handleOAIRecords(w http.ResponseWriter, r *http.Request) {
	if r.PathValue("id") != questDatasetID || g.oaiDataset.LoadError != "" {
		writeError(w, http.StatusNotFound, "OAI 数据不可用")
		return
	}
	offset, err := boundedQueryInt(r, "offset", 0, len(g.oaiDataset.Records))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	limit, err := boundedQueryInt(r, "limit", 100, 500)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	records := g.filteredOAIRecords(r.URL.Query().Get("rnti"))
	if offset > len(records) {
		offset = len(records)
	}
	end := offset + limit
	if end > len(records) {
		end = len(records)
	}
	writeJSON(w, http.StatusOK, map[string]any{"records": records[offset:end], "offset": offset, "total": len(records)})
}

func (g *gateway) handleOAISeries(w http.ResponseWriter, r *http.Request) {
	if r.PathValue("id") != questDatasetID || g.oaiDataset.LoadError != "" {
		writeError(w, http.StatusNotFound, "OAI 数据不可用")
		return
	}
	maxPoints, err := boundedQueryInt(r, "maxPoints", 600, 1000)
	if err != nil || maxPoints < 2 {
		writeError(w, http.StatusBadRequest, "maxPoints必须大于1")
		return
	}
	records := g.filteredOAIRecords(r.URL.Query().Get("rnti"))
	stats := oaiSeriesStats{Count: len(records)}
	for _, record := range records {
		stats.AvgSINRDB += record.SINRDB
		stats.AvgMCS += float64(record.MCS)
		stats.AvgPRBs += float64(record.PRBs)
	}
	if stats.Count > 0 {
		stats.AvgSINRDB /= float64(stats.Count)
		stats.AvgMCS /= float64(stats.Count)
		stats.AvgPRBs /= float64(stats.Count)
	}
	if len(records) <= maxPoints {
		writeJSON(w, http.StatusOK, map[string]any{"points": records, "sourceCount": len(records), "downsampled": false, "stats": stats})
		return
	}
	points := make([]oaiRecord, 0, maxPoints)
	for i := 0; i < maxPoints; i++ {
		index := int(math.Round(float64(i) * float64(len(records)-1) / float64(maxPoints-1)))
		points = append(points, records[index])
	}
	writeJSON(w, http.StatusOK, map[string]any{"points": points, "sourceCount": len(records), "downsampled": true, "stats": stats})
}

func csvSafe(value string) string {
	if value != "" && strings.ContainsRune("=+-@", rune(value[0])) {
		return "'" + value
	}
	return value
}

func (g *gateway) handleOAIExport(w http.ResponseWriter, r *http.Request) {
	if r.PathValue("id") != questDatasetID || g.oaiDataset.LoadError != "" {
		writeError(w, http.StatusNotFound, "OAI 数据不可用")
		return
	}
	records := g.filteredOAIRecords(r.URL.Query().Get("rnti"))
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="quest-oai-records.csv"`)
	writer := csv.NewWriter(w)
	_ = writer.Write([]string{"dataset_id", "source_sha256", "source_line", "record_index", "frame", "slot", "rnti", "sinr_db", "transport_block_bytes", "mcs", "prbs", "pending_ul_bytes", "hol_delay_ms", "ul_delay_ms"})
	for _, record := range records {
		_ = writer.Write([]string{
			csvSafe(g.oaiDataset.ID), csvSafe(g.oaiDataset.SourceSHA), strconv.Itoa(record.SourceLine), strconv.Itoa(record.Index),
			strconv.Itoa(record.Frame), strconv.Itoa(record.Slot), csvSafe(record.RNTI), strconv.FormatFloat(record.SINRDB, 'f', -1, 64),
			strconv.Itoa(record.TransportBlockBytes), strconv.Itoa(record.MCS), strconv.Itoa(record.PRBs), strconv.Itoa(record.PendingULBytes),
			strconv.FormatFloat(record.HOLDelayMS, 'f', -1, 64), strconv.FormatFloat(record.ULDelayMS, 'f', -1, 64),
		})
	}
	writer.Flush()
}
