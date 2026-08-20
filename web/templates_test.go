package web

import (
	"html/template"
	"testing"
)

func TestTemplatesParse(t *testing.T) {
	funcs := template.FuncMap{
		"money":     func(int64) string { return "0.00" },
		"humanDate": func(string) string { return "" },
		"formDT":    func(string) string { return "" },
	}
	if _, err := template.New("").Funcs(funcs).ParseFS(FS, "templates/*.html"); err != nil {
		t.Fatal(err)
	}
}
