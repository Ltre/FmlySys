package store

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestUTF8SummaryTruncatesByRuneWithoutCorruption(t *testing.T) {
	content := "  " + strings.Repeat("家", 99) + "🙂结束  \n  下一段  "
	got := UTF8Summary(content, 100)
	if !utf8.ValidString(got) {
		t.Fatalf("summary is not valid UTF-8: %q", got)
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("truncated summary should end with ellipsis: %q", got)
	}
	if utf8.RuneCountInString(strings.TrimSuffix(got, "...")) != 100 {
		t.Fatalf("summary content should contain exactly 100 runes: %d", utf8.RuneCountInString(strings.TrimSuffix(got, "...")))
	}
	if !strings.Contains(got, "🙂") {
		t.Fatalf("multi-byte rune at the boundary should stay intact: %q", got)
	}
}

func TestUTF8SummaryKeepsShortTextAndCollapsesWhitespace(t *testing.T) {
	if got := UTF8Summary("  第一行\n\t第二行  ", 100); got != "第一行 第二行" {
		t.Fatalf("summary=%q", got)
	}
}
