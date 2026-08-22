package httpserver

import (
	"fmt"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/Ltre/FmlySys/internal/asset"
	"github.com/Ltre/FmlySys/internal/store"
)

const (
	workflowFormMaxBytes = 1 << 20
	workflowBodyMaxBytes = 205 << 20
	workflowMemoryBytes  = 12 << 20
)

// WithAssetWorkflowFixes overrides the affected asset routes while preserving
// the rest of the existing router. It is intentionally layered outside the old
// handlers so existing URLs and authorization rules do not change.
func (s *Server) WithAssetWorkflowFixes(next http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /assets/self-events", s.member("assets.self_change", s.createSelfAssetEventWorkflow))
	mux.HandleFunc("POST /assets/expenses", s.member("expenses.create", s.createExpenseWorkflow))
	mux.HandleFunc("POST /assets/transfers", s.member("transfers.create", s.createTransferWorkflow))
	mux.HandleFunc("POST /assets/reimbursements", s.member("reimbursements.create", s.createReimbursementWorkflow))
	mux.HandleFunc("GET /evidence/{id}", s.memberOrAdmin("assets.view", s.evidencePreviewWorkflow))

	mux.HandleFunc("GET /admin", s.adminOnly(s.adminWorkflow))
	mux.HandleFunc("POST /admin/assets/events", s.adminOnly(s.adminCreateAssetEventWorkflow))
	mux.HandleFunc("POST /admin/assets/expenses", s.adminOnly(s.adminCreateExpenseWorkflow))
	mux.HandleFunc("POST /admin/assets/transfers", s.adminOnly(s.adminCreateTransferWorkflow))
	mux.HandleFunc("POST /admin/assets/reimbursements", s.adminOnly(s.adminCreateReimbursementWorkflow))

	mux.Handle("/", next)
	return mux
}

func parseWorkflowRequest(w http.ResponseWriter, r *http.Request, evidence bool) ([]*multipart.FileHeader, error) {
	contentType := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type")))
	if strings.HasPrefix(contentType, "multipart/") {
		limit := int64(workflowFormMaxBytes)
		memory := int64(workflowFormMaxBytes)
		if evidence {
			limit = workflowBodyMaxBytes
			memory = workflowMemoryBytes
		}
		r.Body = http.MaxBytesReader(w, r.Body, limit)
		if err := r.ParseMultipartForm(memory); err != nil {
			return nil, err
		}
		if !evidence || r.MultipartForm == nil {
			return nil, nil
		}
		files := r.MultipartForm.File["evidence"]
		if err := store.ValidateEvidenceFiles(files); err != nil {
			return nil, err
		}
		return files, nil
	}

	r.Body = http.MaxBytesReader(w, r.Body, workflowFormMaxBytes)
	if err := r.ParseForm(); err != nil {
		return nil, err
	}
	return nil, nil
}

func (s *Server) createSelfAssetEventWorkflow(w http.ResponseWriter, r *http.Request) {
	if _, err := parseWorkflowRequest(w, r, false); err != nil {
		s.fail(w, r, err)
		return
	}
	amount, err := asset.ParseYuan(r.FormValue("amount"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	actor := currentMember(r).ID
	if _, err = s.Store.AddSelfAssetChange(r.Context(), actor, r.FormValue("event_type"), amount, r.FormValue("description"), formDateTime(r.FormValue("occurred_at"))); err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, "/assets")
}

func (s *Server) adminCreateAssetEventWorkflow(w http.ResponseWriter, r *http.Request) {
	if _, err := parseWorkflowRequest(w, r, false); err != nil {
		s.fail(w, r, err)
		return
	}
	amount, err := asset.ParseYuan(r.FormValue("amount"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if _, err = s.Store.AddAssetEventDetailed(r.Context(), s.DevActorID, parseID(r.FormValue("holder_id")), r.FormValue("event_type"), amount, r.FormValue("description"), formDateTime(r.FormValue("occurred_at")), parseID(r.FormValue("related_event_id"))); err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, "/admin")
}

func expenseInputFromWorkflow(r *http.Request, handlerID int64, amount int64) store.ExpenseInputV2 {
	return store.ExpenseInputV2{
		Title:           r.FormValue("title"),
		Category:        r.FormValue("category"),
		AmountCent:      amount,
		OccurredAt:      formDateTime(r.FormValue("occurred_at")),
		HandlerMemberID: handlerID,
		PaymentChannel:  r.FormValue("payment_channel"),
		Merchant:        r.FormValue("merchant"),
		Description:     r.FormValue("description"),
		MatterID:        parseID(r.FormValue("matter_id")),
	}
}

func (s *Server) createExpenseWorkflow(w http.ResponseWriter, r *http.Request) {
	files, err := parseWorkflowRequest(w, r, true)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	amount, err := asset.ParseYuan(r.FormValue("amount"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	actor := currentMember(r).ID
	if _, err := s.Store.CreateExpenseAutoWithEvidence(r.Context(), actor, expenseInputFromWorkflow(r, actor, amount), evidenceDir(s), files); err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, "/assets")
}

func (s *Server) adminCreateExpenseWorkflow(w http.ResponseWriter, r *http.Request) {
	files, err := parseWorkflowRequest(w, r, true)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	amount, err := asset.ParseYuan(r.FormValue("amount"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	in := expenseInputFromWorkflow(r, parseID(r.FormValue("handler_id")), amount)
	if _, err := s.Store.CreateExpenseAutoWithEvidence(r.Context(), s.DevActorID, in, evidenceDir(s), files); err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, "/admin")
}

func (s *Server) createTransferWorkflow(w http.ResponseWriter, r *http.Request) {
	files, err := parseWorkflowRequest(w, r, true)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	amount, err := asset.ParseYuan(r.FormValue("amount"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	actor := currentMember(r).ID
	other := parseID(r.FormValue("counterparty_id"))
	from, to := actor, other
	switch r.FormValue("direction") {
	case "FROM":
		from, to = other, actor
	case "TO":
	default:
		s.fail(w, r, fmt.Errorf("请选择转账方向"))
		return
	}
	id, err := s.Store.CreateTransferV2(r.Context(), actor, from, to, amount, r.FormValue("purpose"), r.FormValue("payment_channel"), formDateTime(r.FormValue("occurred_at")), parseID(r.FormValue("matter_id")))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.SaveEvidenceFiles(r.Context(), actor, "transfer", id, evidenceDir(s), files); err != nil {
		s.fail(w, r, fmt.Errorf("转账已保存，但转账凭证保存失败：%w", err))
		return
	}
	redirect(w, r, "/assets")
}

func (s *Server) adminCreateTransferWorkflow(w http.ResponseWriter, r *http.Request) {
	files, err := parseWorkflowRequest(w, r, true)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	amount, err := asset.ParseYuan(r.FormValue("amount"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	id, err := s.Store.CreateTransferV2(r.Context(), s.DevActorID, parseID(r.FormValue("from_id")), parseID(r.FormValue("to_id")), amount, r.FormValue("purpose"), r.FormValue("payment_channel"), formDateTime(r.FormValue("occurred_at")), parseID(r.FormValue("matter_id")))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.SaveEvidenceFiles(r.Context(), s.DevActorID, "transfer", id, evidenceDir(s), files); err != nil {
		s.fail(w, r, fmt.Errorf("转账已保存，但转账凭证保存失败：%w", err))
		return
	}
	redirect(w, r, "/admin")
}

func (s *Server) createReimbursementWorkflow(w http.ResponseWriter, r *http.Request) {
	s.createReimbursementWorkflowForHolder(w, r, currentMember(r).ID, currentMember(r).ID, "/assets")
}

func (s *Server) adminCreateReimbursementWorkflow(w http.ResponseWriter, r *http.Request) {
	files, err := parseWorkflowRequest(w, r, true)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.createReimbursementWorkflowParsed(w, r, s.DevActorID, parseID(r.FormValue("holder_id")), "/admin", files)
}

func (s *Server) createReimbursementWorkflowForHolder(w http.ResponseWriter, r *http.Request, actor, holder int64, to string) {
	files, err := parseWorkflowRequest(w, r, true)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.createReimbursementWorkflowParsed(w, r, actor, holder, to, files)
}

func (s *Server) createReimbursementWorkflowParsed(w http.ResponseWriter, r *http.Request, actor, holder int64, to string, files []*multipart.FileHeader) {
	amount, err := asset.ParseYuan(r.FormValue("amount"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	id, err := s.Store.CreateReimbursementV2(r.Context(), actor, parseID(r.FormValue("expense_id")), holder, amount, r.FormValue("payment_channel"), formDateTime(r.FormValue("occurred_at")), r.FormValue("note"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.SaveEvidenceFiles(r.Context(), actor, "reimbursement", id, evidenceDir(s), files); err != nil {
		s.fail(w, r, fmt.Errorf("报销已保存，但转账凭证保存失败：%w", err))
		return
	}
	redirect(w, r, to)
}

func evidenceInlineWorkflow(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp", ".heic", ".heif",
		".mp4", ".webm", ".mov", ".m4v", ".mp3", ".m4a", ".aac", ".wav", ".ogg", ".flac",
		".pdf", ".txt":
		return true
	default:
		return false
	}
}

func (s *Server) evidencePreviewWorkflow(w http.ResponseWriter, r *http.Request) {
	path, name, err := s.Store.EvidencePath(r.Context(), parseID(r.PathValue("id")), evidenceDir(s))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if evidenceInlineWorkflow(name) {
		w.Header().Set("Content-Disposition", "inline")
		if strings.EqualFold(filepath.Ext(name), ".txt") {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		}
	} else {
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", strings.ReplaceAll(name, "\"", "")))
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeFile(w, r, path)
}

func (s *Server) adminWorkflow(w http.ResponseWriter, r *http.Request) {
	v := s.base("管理后台")
	v.AdminUsername = currentAdmin(r).Username
	if err := s.populateCommon(r, &v, 0); err != nil {
		s.fail(w, r, err)
		return
	}
	var err error
	v.AdminAssetEvents, err = s.Store.AssetMovementsDetailed(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	v.AssetInflows, err = s.Store.AssetInflowOptions(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	v.PendingJoins, err = s.Store.PendingJoinRequests(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	v.MemberPermissions, err = s.Store.AllMemberPermissions(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, "admin.html", v)
}
