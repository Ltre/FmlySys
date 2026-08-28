package httpserver

import (
	"bytes"
	"html/template"
	"testing"

	"github.com/Ltre/FmlySys/internal/store"
	webassets "github.com/Ltre/FmlySys/web"
)

func TestMedicationViewsRenderSharedNavigation(t *testing.T) {
	funcs := template.FuncMap{
		"money":   func(int64) string { return "0.00" },
		"hasPerm": func(perms map[string]bool, key string) bool { return perms[key] },
	}
	tmpl, err := template.New("").Funcs(funcs).ParseFS(webassets.FS, "templates/dashboard.html")
	if err != nil {
		t.Fatal(err)
	}

	member := store.Member{ID: 1, Name: "测试成员"}
	permissions := map[string]bool{"medication.view": true}
	tests := []struct {
		name string
		view any
	}{
		{name: "medication page", view: medicationPageView{CurrentMember: member, Permissions: permissions}},
		{name: "flat plan list", view: medicationPlansView{CurrentMember: member, Permissions: permissions}},
		{name: "plan detail", view: medicationPlanDetailView{CurrentMember: member, Permissions: permissions}},
		{name: "checkin", view: medicationCheckinView{CurrentMember: member, Permissions: permissions}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			if err := tmpl.ExecuteTemplate(&output, "nav", tt.view); err != nil {
				t.Fatalf("shared nav must render for medication view: %v", err)
			}
		})
	}
}
