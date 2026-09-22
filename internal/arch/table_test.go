package arch

import (
	"bytes"
	"strings"
	"testing"
)

func TestTruncateRows(t *testing.T) {
	rows := []int{1, 2, 3, 4, 5}

	kept, truncated := truncateRows(rows, 2)
	if len(kept) != 2 || truncated != 3 {
		t.Errorf("truncateRows(rows, 2) = %v, %d, want [1 2], 3", kept, truncated)
	}

	kept, truncated = truncateRows(rows, 0)
	if len(kept) != 5 || truncated != 0 {
		t.Errorf("truncateRows(rows, 0) = %v, %d, want all 5 rows, 0 (0 means unlimited)", kept, truncated)
	}

	kept, truncated = truncateRows(rows, 10)
	if len(kept) != 5 || truncated != 0 {
		t.Errorf("truncateRows(rows, 10) = %v, %d, want all 5 rows, 0 (top exceeds row count)", kept, truncated)
	}
}

func TestWriteGateVerdict(t *testing.T) {
	var buf bytes.Buffer
	writeGateVerdict(&buf, 3, 5, "violation", true)
	if !strings.Contains(buf.String(), "PASS: 3 violation(s) is at or under 5") {
		t.Errorf("expected a PASS line, got:\n%s", buf.String())
	}

	buf.Reset()
	writeGateVerdict(&buf, 7, 5, "violation", false)
	if !strings.Contains(buf.String(), "FAIL: 7 violation(s) exceeds 5") {
		t.Errorf("expected a FAIL line, got:\n%s", buf.String())
	}
}
