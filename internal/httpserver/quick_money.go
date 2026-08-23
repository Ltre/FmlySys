package httpserver

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/Ltre/FmlySys/internal/asset"
	"github.com/Ltre/FmlySys/internal/store"
)

type quickMoneyView struct {
	Title           string
	AdminUsername   string
	ActivePartition string
	CurrentMember   store.Member
	Permissions     map[string]bool
	Notes           []store.QuickMoneyNote
	Note            store.QuickMoneyNote
	Members         []store.Member
	Expenses        []store.Expense
	Matters         []store.Matter
}

func (s *Server) WithQuickMoneyNotes(next http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /quick-money-note", s.member("assets.view", s.quickMoneyPage))
	mux.HandleFunc("POST /quick-money-note", s.member("assets.view", s.createQuickMoneyNote))
	mux.HandleFunc("GET /quick-money-note-to-standarized", s.member("assets.view", s.quickMoneyStandardizePage))
	mux.HandleFunc("POST /quick-money-note-to-standarized", s.member("assets.view", s.standardizeQuickMoneyNote))
	mux.Handle("/", next)
	return mux
}

func (s *Server) quickMoneyPage(w http.ResponseWriter, r *http.Request) {
	m := currentMember(r)
	notes, err := s.Store.QuickMoneyNotes(r.Context(), m.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	v := quickMoneyView{
		Title:           "快速记录资金事项",
		ActivePartition: s.PM.ActiveID,
		CurrentMember:   m,
		Permissions:     currentPermissions(r),
		Notes:           notes,
	}
	if err := s.Templates.ExecuteTemplate(w, "quick-money-note.html", v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) createQuickMoneyNote(w http.ResponseWriter, r *http.Request) {
	files, err := parseWorkflowRequest(w, r, true)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	categories := r.Form["category"]
	if len(categories) != 1 {
		s.fail(w, r, fmt.Errorf("快速记录必须且只能选择一个记录分类"))
		return
	}
	actor := currentMember(r).ID
	if _, err := s.Store.CreateQuickMoneyNote(r.Context(), actor, categories[0], r.FormValue("summary"), evidenceDir(s), files); err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, "/quick-money-note")
}

func (s *Server) quickMoneyStandardizePage(w http.ResponseWriter, r *http.Request) {
	m := currentMember(r)
	note, err := s.Store.QuickMoneyNoteByID(r.Context(), parseID(r.URL.Query().Get("id")), m.ID)
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
	v := quickMoneyView{
		Title:           "快速记录 · 数据入库",
		ActivePartition: s.PM.ActiveID,
		CurrentMember:   m,
		Permissions:     currentPermissions(r),
		Note:            note,
		Members:         members,
		Expenses:        expenses,
		Matters:         matters,
	}
	if err := s.Templates.ExecuteTemplate(w, "quick-money-standardize.html", v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func quickMoneyPermission(perms map[string]bool, category string) error {
	var key string
	switch category {
	case store.QuickMoneyExpense:
		key = "expenses.create"
	case store.QuickMoneyTransfer:
		key = "transfers.create"
	case store.QuickMoneyReimbursement:
		key = "reimbursements.create"
	case store.QuickMoneyAssetEvent:
		key = "assets.self_change"
	default:
		return fmt.Errorf("不支持的快速记录分类")
	}
	if !perms[key] {
		return fmt.Errorf("你没有将该类快速记录正式入库的权限")
	}
	return nil
}

func (s *Server) standardizeQuickMoneyNote(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, r, err)
		return
	}
	actor := currentMember(r).ID
	noteID := parseID(r.FormValue("note_id"))
	note, err := s.Store.QuickMoneyNoteByID(r.Context(), noteID, actor)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := quickMoneyPermission(currentPermissions(r), note.Category); err != nil {
		s.fail(w, r, err)
		return
	}
	amount, err := asset.ParseYuan(r.FormValue("amount"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	in := store.QuickMoneyStandardizeInput{
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
	kind, id, err := s.Store.StandardizeQuickMoneyNote(r.Context(), actor, noteID, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	setMoneyRecordHeader(w, kind, id)
	w.Header().Set("X-Fmly-Quick-Note", strconv.FormatInt(noteID, 10))
	redirect(w, r, "/assets")
}
