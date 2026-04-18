package claude

import (
	"bytes"
	"testing"
)

func TestLimitWriter_UnderCap_WritesAll(t *testing.T) {
	var buf bytes.Buffer
	w := limitWriter(&buf, 100)
	n, err := w.Write([]byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if n != 5 {
		t.Errorf("want 5, got %d", n)
	}
	if buf.String() != "hello" {
		t.Errorf("got %q", buf.String())
	}
}

func TestLimitWriter_OverCap_TruncatesButReportsFullWrite(t *testing.T) {
	var buf bytes.Buffer
	w := limitWriter(&buf, 3)
	n, err := w.Write([]byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	// reports full length so subprocess isn't killed
	if n != 5 {
		t.Errorf("want n=5, got %d", n)
	}
	if buf.String() != "hel" {
		t.Errorf("want hel, got %q", buf.String())
	}
}

func TestLimitWriter_SubsequentWritesDropped(t *testing.T) {
	var buf bytes.Buffer
	w := limitWriter(&buf, 3)
	_, _ = w.Write([]byte("hel"))
	_, _ = w.Write([]byte("lo"))
	if buf.String() != "hel" {
		t.Errorf("want hel, got %q", buf.String())
	}
}

func TestLimitWriter_ZeroCapMeansUnlimited(t *testing.T) {
	var buf bytes.Buffer
	w := limitWriter(&buf, 0)
	if w != &buf {
		t.Error("zero cap should return underlying writer unchanged")
	}
}
