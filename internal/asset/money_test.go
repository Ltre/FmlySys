package asset

import "testing"

func TestParseYuan(t *testing.T) {
	cases := map[string]int64{"1": 100, "1.2": 120, "1.23": 123, "0.01": 1, "-2.50": -250}
	for in, want := range cases {
		got, err := ParseYuan(in)
		if err != nil || got != want {
			t.Fatalf("%s got %d err=%v want %d", in, got, err, want)
		}
	}
	if _, err := ParseYuan("1.234"); err == nil {
		t.Fatal("expected precision error")
	}
}
