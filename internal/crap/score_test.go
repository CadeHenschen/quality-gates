package crap

import (
	"reflect"
	"testing"
)

func TestScore(t *testing.T) {
	cases := []struct {
		name string
		fn   Function
		want float64
	}{
		{
			name: "fully covered, complexity 1",
			fn:   Function{Complexity: 1, LinesTotal: 10, LinesCovered: 10},
			want: 1, // 1^2 * 0^3 + 1
		},
		{
			name: "zero coverage, complexity 10",
			fn:   Function{Complexity: 10, LinesTotal: 10, LinesCovered: 0},
			want: 110, // 10^2 * 1^3 + 10
		},
		{
			name: "half covered, complexity 4",
			fn:   Function{Complexity: 4, LinesTotal: 10, LinesCovered: 5},
			want: 6, // 4^2 * 0.5^3 + 4 = 16*0.125+4 = 2+4
		},
		{
			name: "no coverable lines treated as fully covered",
			fn:   Function{Complexity: 3, LinesTotal: 0, LinesCovered: 0},
			want: 3, // 3^2 * 0^3 + 3
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Score(c.fn)
			if got != c.want {
				t.Errorf("Score(%+v) = %v, want %v", c.fn, got, c.want)
			}
		})
	}
}

func TestNewReportSortsWorstFirstAndGates(t *testing.T) {
	fns := []Function{
		{Name: "low", Complexity: 1, LinesTotal: 10, LinesCovered: 10},  // crap 1
		{Name: "high", Complexity: 10, LinesTotal: 10, LinesCovered: 0}, // crap 110
		{Name: "mid", Complexity: 4, LinesTotal: 10, LinesCovered: 5},   // crap 6
	}

	report := NewReport(fns, 30)

	if len(report.Functions) != 3 {
		t.Fatalf("got %d functions, want 3", len(report.Functions))
	}
	order := []string{report.Functions[0].Name, report.Functions[1].Name, report.Functions[2].Name}
	want := []string{"high", "mid", "low"}
	for i := range want {
		if order[i] != want[i] {
			t.Errorf("position %d = %q, want %q (full order: %v)", i, order[i], want[i], order)
		}
	}

	if report.Passed {
		t.Error("report.Passed = true, want false (high exceeds fail-above 30)")
	}
	if report.ExitCode() != 1 {
		t.Errorf("ExitCode() = %d, want 1", report.ExitCode())
	}

	passing := NewReport(fns, 200)
	if !passing.Passed {
		t.Error("report.Passed = false, want true (fail-above 200 exceeds every score)")
	}
	if passing.ExitCode() != 0 {
		t.Errorf("ExitCode() = %d, want 0", passing.ExitCode())
	}
}

func TestMergeLineRanges(t *testing.T) {
	cases := []struct {
		name  string
		lines []int
		want  []LineRange
	}{
		{"empty", nil, nil},
		{"single", []int{5}, []LineRange{{5, 5}}},
		{"consecutive", []int{19, 20, 21, 25}, []LineRange{{19, 21}, {25, 25}}},
		{"unordered input", []int{25, 19, 21, 20}, []LineRange{{19, 21}, {25, 25}}},
		{"duplicates", []int{5, 5, 6, 6, 6}, []LineRange{{5, 6}}},
		{"all separate", []int{1, 3, 5}, []LineRange{{1, 1}, {3, 3}, {5, 5}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := MergeLineRanges(c.lines)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("MergeLineRanges(%v) = %v, want %v", c.lines, got, c.want)
			}
		})
	}
}
