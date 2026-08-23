package httpserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Ltre/FmlySys/internal/asset"
	"github.com/Ltre/FmlySys/internal/store"
)

type adminQuickMoneyEvidenceJSON struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type adminQuickMoneyNoteJSON struct {
	ID               int64                         `json:"id"`
	Category         string                        `json:"category"`
	CategoryLabel    string                        `json:"category_label"`
	Summary          string                        `json:"summary"`
	Status           string                        `json:"status"`
	CreatedBy        int64                         `json:"created_by"`
	CreatorName      string                        `json:"creator_name"`
	CreatedAt        string                        `json:"created_at"`
	StandardizedType string                        `json:"standardized_type,omitempty"`
	StandardizedID   int64                         `json:"standardized_id,omitempty"`
	StandardizedAt   string                        `json:"standardized_at,omitempty"`
	Evidence         []adminQuickMoneyEvidenceJSON `json:"evidence,omitempty"`
}

type adminTransferJSON struct {
	ID          int64  `json:"id"`
	Purpose     string `json:"purpose"`
	MatterTitle string `json:"matter_title"`
}

type adminQuickMoneyStandardizeView struct {
	Title         string
	AdminUsername string
	Note          store.AdminQuickMoneyNote
	Owner         store.Member
	Members       []store.Member
	Expenses      []store.Expense
	Matters       []store.Matter
}

type adminBufferedResponse struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func newAdminBufferedResponse() *adminBufferedResponse {
	return &adminBufferedResponse{header: make(http.Header), status: http.StatusOK}
}

func (w *adminBufferedResponse) Header() http.Header         { return w.header }
func (w *adminBufferedResponse) WriteHeader(status int)      { w.status = status }
func (w *adminBufferedResponse) Write(p []byte) (int, error) { return w.body.Write(p) }

func copyAdminBufferedResponse(dst http.ResponseWriter, src *adminBufferedResponse) {
	for key, values := range src.header {
		for _, value := range values {
			dst.Header().Add(key, value)
		}
	}
	dst.Header().Del("Content-Length")
	dst.WriteHeader(src.status)
	_, _ = dst.Write(src.body.Bytes())
}

func (s *Server) adminPageWithEnhancements(next http.Handler, w http.ResponseWriter, r *http.Request) {
	buffered := newAdminBufferedResponse()
	next.ServeHTTP(buffered, r)
	if buffered.status == http.StatusOK && strings.Contains(strings.ToLower(buffered.body.String()), "</body>") {
		buffered.header.Set("Content-Type", "text/html; charset=utf-8")
		body := buffered.body.String()
		const script = `<script src="/static/admin-enhancements.js" defer></script>`
		if !strings.Contains(body, script) {
			body = strings.Replace(body, "</body>", script+"</body>", 1)
			buffered.body.Reset()
			_, _ = buffered.body.WriteString(body)
		}
	}
	copyAdminBufferedResponse(w, buffered)
}

func (s *Server) WithAdminEnhancements(next http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /admin", func(w http.ResponseWriter, r *http.Request) { s.adminPageWithEnhancements(next, w, r) })
	mux.HandleFunc("POST /admin/members/{id}/edit", s.adminOnly(s.adminUpdateMemberInfo))
	mux.HandleFunc("POST /admin/members/{id}/delete", s.adminOnly(s.adminSoftDeleteMember))
	mux.HandleFunc("GET /admin/api/quick-money-notes", s.adminOnly(s.adminQuickMoneyNotesJSON))
	mux.HandleFunc("GET /admin/api/transfers", s.adminOnly(s.adminTransfersJSON))
	mux.HandleFunc("GET /admin/quick-money-note-to-standarized", s.adminOnly(s.adminQuickMoneyStandardizePage))
	mux.HandleFunc("POST /admin/quick-money-note-to-standarized", s.adminOnly(s.adminQuickMoneyStandardize))
	mux.Handle("/", next)
	return mux
}

func (s *Server) adminUpdateMemberInfo(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.UpdateMemberInfo(r.Context(), s.DevActorID, parseID(r.PathValue("id")), r.FormValue("name"), r.FormValue("relation")); err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, "/admin#members-and-permissions")
}

func (s *Server) adminSoftDeleteMember(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.SoftDeleteMember(r.Context(), s.DevActorID, parseID(r.PathValue("id"))); err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, "/admin#members-and-permissions")
}

func (s *Server) adminQuickMoneyNotesJSON(w http.ResponseWriter, r *http.Request) {
	notes, err := s.Store.AdminQuickMoneyNotes(r.Context())
	if err != nil {
		passkeyJSONError(w, err, http.StatusBadRequest)
		return
	}
	items := make([]adminQuickMoneyNoteJSON, 0, len(notes))
	for _, note := range notes {
		item := adminQuickMoneyNoteJSON{
			ID:               note.ID,
			Category:         note.Category,
			CategoryLabel:    note.CategoryLabel,
			Summary:          note.Summary,
			Status:           note.Status,
			CreatedBy:        note.CreatedBy,
			CreatorName:      note.CreatorName,
			CreatedAt:        note.CreatedAt,
			StandardizedType: note.StandardizedEntityType,
			StandardizedID:   note.StandardizedEntityID,
			StandardizedAt:   note.StandardizedAt,
		}
		for _, evidence := range note.Evidence {
			item.Evidence = append(item.Evidence, adminQuickMoneyEvidenceJSON{ID: evidence.ID, Name: evidence.OriginalName})
		}
		items = append(items, item)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "notes": items})
}

func (s *Server) adminTransfersJSON(w http.ResponseWriter, r *http.Request) {
	transfers, err := s.Store.Transfers(r.Context())
	if err != nil {
		passkeyJSONError(w, err, http.StatusBadRequest)
		return
	}
	items := make([]adminTransferJSON, 0, len(transfers))
	for _, transfer := range transfers {
		items = append(items, adminTransferJSON{ID: transfer.ID, Purpose: transfer.Purpose, MatterTitle: transfer.MatterTitle})
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "transfers": items})
}

func (s *Server) adminQuickMoneyStandardizePage(w http.ResponseWriter, r *http.Request) {
	note, err := s.Store.AdminQuickMoneyNoteByID(r.Context(), parseID(r.URL.Query().Get("id")))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	owner, err := s.Store.MemberByID(r.Context(), note.CreatedBy)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	members, err := s.familyMembers(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	expenses, err := s.Store.ExpensesV2(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	matters, err := s.Store.Matters(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	v := adminQuickMoneyStandardizeView{
		Title:         "后台处理快速记录",
		AdminUsername: currentAdmin(r).Username,
		Note:          note,
		Owner:         owner,
		Members:       members,
		Expenses:      expenses,
		Matters:       matters,
	}
	if err := s.Templates.ExecuteTemplate(w, "admin-quick-money-standardize.html", v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func quickMoneyStandardizeInputFromRequest(r *http.Request, amount int64) store.QuickMoneyStandardizeInput {
	return store.QuickMoneyStandardizeInput{
		Title:           r.FormValue("title"),
		ExpenseCategory: r.FormValue("expense_category"),
		AmountCent:      amount,
		OccurredAt:      formDateTime(r.FormValue("occurred_at")),
		PaymentChannel:  r.FormValue("payment_channel"),
		Merchant:        r.FormValue("merchant"),
		Description:     r.FormValue("description"),
		MatterID:        parseID(r.FormValue("matter_id")),
		Direction:       r.FormValue("direction"),
		CounterpartyID:  parseID(r.FormValue("counterparty_id")),
		Purpose:         r.FormValue("purpose"),
		ExpenseID:       parseID(r.FormValue("expense_id")),
		Note:            r.FormValue("note"),
		EventType:       r.FormValue("event_type"),
	}
}

func (s *Server) adminQuickMoneyStandardize(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, r, err)
		return
	}
	noteID := parseID(r.FormValue("note_id"))
	note, err := s.Store.AdminQuickMoneyNoteByID(r.Context(), noteID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	amount, err := asset.ParseYuan(r.FormValue("amount"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	kind, id, err := s.Store.StandardizeQuickMoneyNoteForOwner(r.Context(), note.CreatedBy, s.DevActorID, noteID, quickMoneyStandardizeInputFromRequest(r, amount))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	setMoneyRecordHeader(w, kind, id)
	redirect(w, r, "/admin#admin-quick-money-notes")
}
