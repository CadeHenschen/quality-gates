package reportio

import (
	"bytes"
	"testing"
)

type sample struct {
	Name string `json:"name"`
	N    int    `json:"n"`
}

func TestWriteJSONReadReportRoundTrip(t *testing.T) {
	want := sample{Name: "hotspot", N: 7}

	var buf bytes.Buffer
	if err := WriteJSON(&buf, want); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	got, err := ReadReport[sample](&buf)
	if err != nil {
		t.Fatalf("ReadReport: %v", err)
	}
	if got != want {
		t.Errorf("ReadReport = %+v, want %+v", got, want)
	}
}

func TestExitCode(t *testing.T) {
	if got := ExitCode(true); got != 0 {
		t.Errorf("ExitCode(true) = %d, want 0", got)
	}
	if got := ExitCode(false); got != 1 {
		t.Errorf("ExitCode(false) = %d, want 1", got)
	}
}
