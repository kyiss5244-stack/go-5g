package main

import (
	"encoding/binary"
	"testing"
)

func dockerFrame(stream byte, payload string) []byte {
	frame := make([]byte, 8+len(payload))
	frame[0] = stream
	binary.BigEndian.PutUint32(frame[4:8], uint32(len(payload)))
	copy(frame[8:], payload)
	return frame
}

func TestDemultiplexLogs(t *testing.T) {
	data := append(dockerFrame(1, "first\n"), dockerFrame(2, "second\nthird\n")...)
	got := demultiplexLogs(data)
	want := []string{"first", "second", "third"}
	if len(got) != len(want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestDemultiplexPlainTTYLogs(t *testing.T) {
	got := demultiplexLogs([]byte("one\ntwo\n"))
	if len(got) != 2 || got[0] != "one" || got[1] != "two" {
		t.Fatalf("got %q", got)
	}
}
