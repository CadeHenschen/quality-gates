package tokenizers

import (
	"reflect"
	"testing"

	"git.roost-r.com/cadeh/quality-gates/internal/dupe"
)

func TestToFileTokens(t *testing.T) {
	raw := []RawFileTokens{
		{
			File: "a.py",
			Tokens: []struct {
				Text string `json:"text"`
				Line int    `json:"line"`
			}{
				{Text: "def", Line: 1},
				{Text: "f", Line: 1},
			},
		},
	}

	got := ToFileTokens(raw)
	want := []dupe.FileTokens{
		{File: "a.py", Tokens: []dupe.Token{{Text: "def", Line: 1}, {Text: "f", Line: 1}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ToFileTokens(%+v) = %+v, want %+v", raw, got, want)
	}
}

func TestToFileTokensEmpty(t *testing.T) {
	got := ToFileTokens(nil)
	if len(got) != 0 {
		t.Errorf("ToFileTokens(nil) = %+v, want empty", got)
	}
}
