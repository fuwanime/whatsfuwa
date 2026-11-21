package app

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestWrapTextKeepsEveryLineWithinWidth(t *testing.T) {
	lines := wrapText("Ini pesan yang cukup panjang untuk dipotong menjadi beberapa baris tanpa merusak karakter 日本語.", 20)
	if len(lines) < 2 {
		t.Fatalf("expected wrapped lines, got %#v", lines)
	}
	for _, line := range lines {
		if utf8.RuneCountInString(line) > 20 {
			t.Fatalf("line exceeds width: %q", line)
		}
	}
}

func TestTruncateUnicode(t *testing.T) {
	got := truncate("FuwaStream 日本語", 9)
	if utf8.RuneCountInString(got) != 9 || !strings.HasSuffix(got, "…") {
		t.Fatalf("unexpected truncation: %q", got)
	}
}

func TestDigitsOnly(t *testing.T) {
	if got := digitsOnly("+62 812-3456-7890"); got != "6281234567890" {
		t.Fatalf("unexpected phone normalization: %q", got)
	}
}

func TestTrimLastRune(t *testing.T) {
	if got := trimLastRune("halo界"); got != "halo" {
		t.Fatalf("unexpected backspace result: %q", got)
	}
}
