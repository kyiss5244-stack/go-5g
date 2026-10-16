package main

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseQuestCSVMapsPatchedOAIColumns(t *testing.T) {
	records, err := parseQuestCSV(strings.NewReader("878,9,46dd,21.5,421,9,18,419,6.977,0\n884,19,46dd,22.5,1345,9,59,1330,9.392,0\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("record count = %d, want 2", len(records))
	}
	got := records[0]
	if got.SourceLine != 1 || got.Frame != 878 || got.Slot != 9 || got.RNTI != "46dd" || got.SINRDB != 21.5 ||
		got.TransportBlockBytes != 421 || got.MCS != 9 || got.PRBs != 18 || got.PendingULBytes != 419 || got.HOLDelayMS != 6.977 || got.ULDelayMS != 0 {
		t.Fatalf("unexpected record: %+v", got)
	}
}

func TestParseQuestCSVRejectsMalformedRows(t *testing.T) {
	for _, input := range []string{
		"878,9,46dd,21.5,421,9,18,419,6.977\n",
		"878,9,46dd,NaN,421,9,18,419,6.977,0\n",
		"878,9,,21.5,421,9,18,419,6.977,0\n",
		"878,9,46dd,21.5,421,9,18,-1,6.977,0\n",
	} {
		if _, err := parseQuestCSV(strings.NewReader(input)); err == nil {
			t.Fatalf("invalid row accepted: %q", input)
		}
	}
}

func TestOAISeriesFiltersAndDownsamples(t *testing.T) {
	dataset := questDatasetMetadata()
	for i := 0; i < 10; i++ {
		rnti := "a"
		if i%2 == 1 {
			rnti = "b"
		}
		dataset.Records = append(dataset.Records, oaiRecord{Index: i, SourceLine: i + 1, RNTI: rnti, SINRDB: float64(i)})
	}
	g := &gateway{oaiDataset: dataset}
	request := httptest.NewRequest("GET", "/api/oai/datasets/quest-users2-run1/series?rnti=a&maxPoints=3", nil)
	request.SetPathValue("id", questDatasetID)
	recorder := httptest.NewRecorder()
	g.handleOAISeries(recorder, request)
	if recorder.Code != 200 {
		t.Fatalf("status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	if !strings.Contains(body, `"sourceCount":5`) || !strings.Contains(body, `"downsampled":true`) || !strings.Contains(body, `"index":8`) || !strings.Contains(body, `"avgSinrDb":4`) {
		t.Fatalf("unexpected series response: %s", body)
	}
}

func TestOAIExportFiltersAndIncludesProvenance(t *testing.T) {
	dataset := questDatasetMetadata()
	dataset.Records = []oaiRecord{
		{Index: 0, SourceLine: 1, Frame: 878, Slot: 9, RNTI: "46dd", SINRDB: 21.5, TransportBlockBytes: 421, MCS: 9, PRBs: 18, PendingULBytes: 419, HOLDelayMS: 6.977},
		{Index: 1, SourceLine: 2, Frame: 879, Slot: 9, RNTI: "0dd4", SINRDB: 25},
	}
	g := &gateway{oaiDataset: dataset}
	request := httptest.NewRequest("GET", "/api/oai/datasets/quest-users2-run1/export?rnti=46dd", nil)
	request.SetPathValue("id", questDatasetID)
	recorder := httptest.NewRecorder()
	g.handleOAIExport(recorder, request)
	if recorder.Code != 200 || recorder.Header().Get("Content-Type") != "text/csv; charset=utf-8" {
		t.Fatalf("unexpected response: status=%d content-type=%q", recorder.Code, recorder.Header().Get("Content-Type"))
	}
	body := recorder.Body.String()
	if !strings.Contains(body, "dataset_id,source_sha256,source_line") || !strings.Contains(body, questSourceSHA) || !strings.Contains(body, ",46dd,") || strings.Contains(body, ",0dd4,") {
		t.Fatalf("unexpected export: %s", body)
	}
}

func TestCSVSafeEscapesSpreadsheetFormulaPrefix(t *testing.T) {
	if got := csvSafe("=1+1"); got != "'=1+1" {
		t.Fatalf("csvSafe formula = %q", got)
	}
	if got := csvSafe("46dd"); got != "46dd" {
		t.Fatalf("csvSafe RNTI = %q", got)
	}
}
