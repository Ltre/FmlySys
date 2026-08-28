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
		"hasPerm":   func(map[string]bool, string) bool { return false },
		"canManageCreated": func(map[string]bool, int64, int64, string) bool {
			return false
		},
		"memberHasPerm": func(map[int64]map[string]bool, int64, string) bool { return false },
		"defaultPerm":   func(string) bool { return false },
	}
	if _, err := template.New("").Funcs(funcs).ParseFS(FS, "templates/*.html"); err != nil {
		t.Fatal(err)
	}
}
