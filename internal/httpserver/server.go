package httpserver

import (
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Ltre/FmlySys/internal/asset"
	"github.com/Ltre/FmlySys/internal/partition"
	"github.com/Ltre/FmlySys/internal/store"
	webassets "github.com/Ltre/FmlySys/web"
)

type Server struct {
	PM        *partition.Manager
	Store     *store.Store
	ActorID   int64
	Templates *template.Template
	mux       *http.ServeMux
}

func New(pm *partition.Manager, st *store.Store, actorID int64) (*Server, error) {
	funcs := template.FuncMap{
		"money": asset.FormatYuan,
		"humanDate": func(v string) string {
			if len(v) >= 10 {
				return v[:10]
			}
			return v
		},
		"formDT": func(v string) string {
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				return t.Format("2006-01-02T15:04")
			}
			return ""
		},
	}
	t, err := template.New("").Funcs(funcs).ParseFS(webassets.FS, "templates/*.html")
	if err != nil {
		return nil, err
	}
	s := &Server{PM: pm, Store: st, ActorID: actorID, Templates: t, mux: http.NewServeMux()}
	s.routes()
	return s, nil
}

func (s *Server) Handler() http.Handler { return logging(s.mux) }

func (s *Server) routes() {
	staticFS, _ := fs.Sub(webassets.FS, "static")
	s.mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
	s.mux.HandleFunc("GET /", s.dashboard)
	s.mux.HandleFunc("GET /assets", s.assets)
	s.mux.HandleFunc("POST /members", s.createMember)
	s.mux.HandleFunc("POST /assets/events", s.createAssetEvent)
	s.mux.HandleFunc("POST /assets/expenses", s.createExpense)
	s.mux.HandleFunc("GET /assets/expenses/{id}/edit", s.editExpense)
	s.mux.HandleFunc("POST /assets/expenses/{id}", s.updateExpense)
	s.mux.HandleFunc("POST /assets/transfers", s.createTransfer)
	s.mux.HandleFunc("POST /assets/reimbursements", s.createReimbursement)
	s.mux.HandleFunc("GET /matters", s.matters)
	s.mux.HandleFunc("POST /matters", s.createMatter)
	s.mux.HandleFunc("POST /matters/{id}/status", s.setMatterStatus)
	s.mux.HandleFunc("GET /share", s.share)
	s.mux.HandleFunc("POST /share", s.createArchive)
	s.mux.HandleFunc("POST /share/{id}/attachments", s.uploadArchive)
	s.mux.HandleFunc("GET /files/{id}", s.file)
	s.mux.HandleFunc("GET /admin", s.admin)
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("ok\n"))
	})
}

type view struct {
	Title           string
	ActivePartition string
	Summary         store.AssetSummary
	Members         []store.Member
	Expenses        []store.Expense
	Expense         store.Expense
	AssetEvents     []store.AssetEvent
	Transfers       []store.Transfer
	Reimbursements  []store.Reimbursement
	Matters         []store.Matter
	Archives        []store.Archive
	Error           string
}

func (s *Server) base(title string) view { return view{Title: title, ActivePartition: s.PM.ActiveID} }
func (s *Server) render(w http.ResponseWriter, name string, v view) {
	if err := s.Templates.ExecuteTemplate(w, name, v); err != nil {
		http.Error(w, err.Error(), 500)
	}
}
func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	http.Error(w, err.Error(), http.StatusBadRequest)
}
func redirect(w http.ResponseWriter, r *http.Request, to string) {
	http.Redirect(w, r, to, http.StatusSeeOther)
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	v := s.base("FmlySys")
	var err error
	v.Summary, err = s.Store.AssetSummary(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	v.Matters, err = s.Store.Matters(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	v.Archives, err = s.Store.Archives(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if len(v.Matters) > 6 {
		v.Matters = v.Matters[:6]
	}
	if len(v.Archives) > 6 {
		v.Archives = v.Archives[:6]
	}
	s.render(w, "dashboard.html", v)
}
func (s *Server) assets(w http.ResponseWriter, r *http.Request) {
	v := s.base("公共资产")
	var err error
	v.Summary, err = s.Store.AssetSummary(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	v.Members, err = s.Store.Members(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	v.Expenses, err = s.Store.Expenses(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	v.AssetEvents, err = s.Store.AssetEvents(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	v.Transfers, err = s.Store.Transfers(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	v.Reimbursements, err = s.Store.Reimbursements(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	v.Matters, err = s.Store.Matters(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, "assets.html", v)
}
func (s *Server) matters(w http.ResponseWriter, r *http.Request) {
	v := s.base("家族事务")
	var err error
	v.Members, err = s.Store.Members(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	v.Matters, err = s.Store.Matters(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, "matters.html", v)
}
func (s *Server) share(w http.ResponseWriter, r *http.Request) {
	v := s.base("信息共享")
	var err error
	v.Archives, err = s.Store.Archives(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, "share.html", v)
}
func (s *Server) admin(w http.ResponseWriter, r *http.Request) {
	v := s.base("管理后台")
	s.render(w, "admin.html", v)
}

func (s *Server) createMember(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.CreateMember(r.Context(), s.ActorID, r.FormValue("name"), r.FormValue("relation")); err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, "/assets")
}
func (s *Server) createAssetEvent(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, r, err)
		return
	}
	amount, err := asset.ParseYuan(r.FormValue("amount"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	holder, _ := strconv.ParseInt(r.FormValue("holder_id"), 10, 64)
	if err := s.Store.AddAssetEvent(r.Context(), s.ActorID, holder, r.FormValue("event_type"), amount, r.FormValue("description"), formDateTime(r.FormValue("occurred_at"))); err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, "/assets")
}
func (s *Server) createExpense(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, r, err)
		return
	}
	amount, err := asset.ParseYuan(r.FormValue("amount"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	in := store.ExpenseInput{Title: r.FormValue("title"), Category: r.FormValue("category"), AmountCent: amount, OccurredAt: formDateTime(r.FormValue("occurred_at")), HandlerMemberID: parseID(r.FormValue("handler_id")), PayerMemberID: parseID(r.FormValue("payer_id")), FundingType: r.FormValue("funding_type"), HolderMemberID: parseID(r.FormValue("holder_id")), PaymentChannel: r.FormValue("payment_channel"), Merchant: r.FormValue("merchant"), Description: r.FormValue("description"), MatterID: parseID(r.FormValue("matter_id"))}
	if err := s.Store.CreateExpense(r.Context(), s.ActorID, in); err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, "/assets")
}
func (s *Server) editExpense(w http.ResponseWriter, r *http.Request) {
	v := s.base("编辑公共消费")
	var err error
	v.Expense, err = s.Store.ExpenseByID(r.Context(), parseID(r.PathValue("id")))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	v.Matters, err = s.Store.Matters(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, "expense-edit.html", v)
}

func (s *Server) updateExpense(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, r, err)
		return
	}
	amount, err := asset.ParseYuan(r.FormValue("amount"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	in := store.ExpenseUpdate{Title: r.FormValue("title"), Category: r.FormValue("category"), AmountCent: amount, OccurredAt: formDateTime(r.FormValue("occurred_at")), PaymentChannel: r.FormValue("payment_channel"), Merchant: r.FormValue("merchant"), Description: r.FormValue("description"), MatterID: parseID(r.FormValue("matter_id"))}
	if err := s.Store.UpdateExpense(r.Context(), s.ActorID, parseID(r.PathValue("id")), in); err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, "/assets")
}

func (s *Server) createTransfer(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, r, err)
		return
	}
	amount, err := asset.ParseYuan(r.FormValue("amount"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.CreateTransfer(r.Context(), s.ActorID, parseID(r.FormValue("from_id")), parseID(r.FormValue("to_id")), amount, r.FormValue("purpose"), r.FormValue("payment_channel"), formDateTime(r.FormValue("occurred_at")), parseID(r.FormValue("matter_id"))); err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, "/assets")
}
func (s *Server) createReimbursement(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, r, err)
		return
	}
	amount, err := asset.ParseYuan(r.FormValue("amount"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.CreateReimbursement(r.Context(), s.ActorID, parseID(r.FormValue("expense_id")), parseID(r.FormValue("holder_id")), amount, r.FormValue("payment_channel"), formDateTime(r.FormValue("occurred_at")), r.FormValue("note")); err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, "/assets")
}
func (s *Server) createMatter(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, r, err)
		return
	}
	in := store.MatterInput{ParentID: parseID(r.FormValue("parent_id")), Title: r.FormValue("title"), Type: r.FormValue("type"), Description: r.FormValue("description"), Status: r.FormValue("status"), StartDate: r.FormValue("start_date"), DueDate: r.FormValue("due_date"), OwnerMemberID: parseID(r.FormValue("owner_id"))}
	if err := s.Store.CreateMatter(r.Context(), s.ActorID, in); err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, "/matters")
}
func (s *Server) setMatterStatus(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, r, err)
		return
	}
	id := parseID(r.PathValue("id"))
	if err := s.Store.SetMatterStatus(r.Context(), s.ActorID, id, r.FormValue("status")); err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, "/matters")
}
func (s *Server) createArchive(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, r, err)
		return
	}
	if _, err := s.Store.CreateArchive(r.Context(), s.ActorID, r.FormValue("title"), r.FormValue("category"), r.FormValue("content"), r.FormValue("visibility")); err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, "/share")
}
func (s *Server) uploadArchive(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(52 << 20); err != nil {
		s.fail(w, r, err)
		return
	}
	file := r.MultipartForm.File["file"]
	if len(file) == 0 {
		s.fail(w, r, fmt.Errorf("请选择附件"))
		return
	}
	if err := s.Store.SaveArchiveAttachment(r.Context(), s.ActorID, parseID(r.PathValue("id")), filepath.Join(s.PM.ActiveDir, "uploads"), file[0]); err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, "/share")
}
func (s *Server) file(w http.ResponseWriter, r *http.Request) {
	path, name, err := s.Store.AttachmentPath(r.Context(), parseID(r.PathValue("id")), filepath.Join(s.PM.ActiveDir, "uploads"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", strings.ReplaceAll(name, "\"", "")))
	http.ServeFile(w, r, path)
}
func parseID(v string) int64 { n, _ := strconv.ParseInt(v, 10, 64); return n }
func formDateTime(v string) string {
	if v == "" {
		return time.Now().UTC().Format(time.RFC3339)
	}
	if t, err := time.Parse("2006-01-02T15:04", v); err == nil {
		return t.UTC().Format(time.RFC3339)
	}
	return v
}
func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}
