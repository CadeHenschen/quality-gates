package dupe

import "testing"

func TestNewReportComputesPercentAndGate(t *testing.T) {
	files := []FileTokens{
		{File: "a.go", Tokens: toks("1", "2", "3", "4", "5", "6", "7", "8", "9", "10")}, // 10 lines
		{File: "b.go", Tokens: toks("1", "2", "3", "4", "5")},                           // 5 lines
	}
	clones := []Clone{
		{FileA: "a.go", StartLineA: 1, EndLineA: 2, FileB: "b.go", StartLineB: 1, EndLineB: 2}, // 2+2=4 dup lines
	}

	report := NewReport(files, clones, 30) // 26.67% duplication, so 30% passes
	if report.TotalLines != 15 {
		t.Errorf("TotalLines = %d, want 15", report.TotalLines)
	}
	if report.DuplicatedLines != 4 {
		t.Errorf("DuplicatedLines = %d, want 4", report.DuplicatedLines)
	}
	wantPct := 4.0 / 15.0 * 100
	if report.DuplicationPercent != wantPct {
		t.Errorf("DuplicationPercent = %v, want %v", report.DuplicationPercent, wantPct)
	}
	if !report.Passed {
		t.Errorf("Passed = false, want true (%.2f%% <= 30%%)", report.DuplicationPercent)
	}
	if report.ExitCode() != 0 {
		t.Errorf("ExitCode() = %d, want 0", report.ExitCode())
	}

	failing := NewReport(files, clones, 1)
	if failing.Passed {
		t.Errorf("Passed = true, want false (%.2f%% > 1%%)", failing.DuplicationPercent)
	}
	if failing.ExitCode() != 1 {
		t.Errorf("ExitCode() = %d, want 1", failing.ExitCode())
	}
}

func TestNewReportNoFilesNoDivideByZero(t *testing.T) {
	report := NewReport(nil, nil, 5)
	if report.TotalLines != 0 || report.DuplicationPercent != 0 {
		t.Errorf("report = %+v, want zero totals", report)
	}
	if !report.Passed {
		t.Error("Passed = false, want true (0%% duplication always passes)")
	}
}
