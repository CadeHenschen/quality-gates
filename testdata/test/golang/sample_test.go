package sample

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestGood(t *testing.T) {
	if got := 1 + 1; got != 2 {
		t.Errorf("got %d", got)
	}
	assert.Equal(t, 2, 1+1)
}

func TestNoAssertions(t *testing.T) {
	_ = 1 + 1
}

func TestSetupHelperOnlyIsNotAnAssertion(t *testing.T) {
	newFixture(t)
}

func TestDelegatesToHelper(t *testing.T) {
	assertSomething(t, 1)
}

func TestSubtests(t *testing.T) {
	t.Run("a", func(t *testing.T) {
		t.Fatal("boom")
	})
}

func TestUnconditionalSkip(t *testing.T) {
	t.Skip("later")
	assert.True(t, true)
}

func TestConditionalSkipIsFine(t *testing.T) {
	if os.Getenv("TOOL") == "" {
		t.Skip("tool missing")
	}
	t.Error("x")
}

func TestOnlyMockAssertions(t *testing.T) {
	m := &mock.Mock{}
	m.AssertExpectations(t)
	m.AssertCalled(t, "Do")
}

func TestMockPlusBehavior(t *testing.T) {
	m := &mock.Mock{}
	m.AssertCalled(t, "Do")
	assert.Equal(t, 1, 1)
}

func TestTempNoCleanup(t *testing.T) {
	dir, _ := os.MkdirTemp("", "x")
	_ = dir
	t.Error("x")
}

func TestTempWithDefer(t *testing.T) {
	dir, _ := os.MkdirTemp("", "x")
	defer os.RemoveAll(dir)
	t.Error("x")
}

func TestTempWithCleanup(t *testing.T) {
	f, _ := os.CreateTemp("", "x")
	t.Cleanup(func() { os.Remove(f.Name()) })
	t.Error("x")
}

func Testlowercase(t *testing.T) {}

func TestHelperNotATest(t *testing.T, extra int) {}

func BenchmarkNotATest(b *testing.B) {}

func TestSwitchSkipIsConditional(t *testing.T) {
	switch {
	case os.Getenv("X") == "":
		t.Skip("no X")
	}
	t.Error("x")
}

func TestSelectAndTypeSwitchDontCrash(t *testing.T) {
	var v any
	switch v.(type) {
	case int:
		t.Error("int")
	}
	select {
	default:
		t.Error("default")
	}
}

func TestAssertsViaUnnamedHelper(t *testing.T) {
	locate(t, "x")
}

func TestAssertsViaHelperOfHelper(t *testing.T) {
	outer(t)
}

// locate's name doesn't sound like an assertion, but its body fails t.
func locate(t *testing.T, name string) {
	t.Helper()
	if name == "" {
		t.Fatalf("missing")
	}
}

func outer(t *testing.T) { locate(t, "y") }
