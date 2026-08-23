package httpserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/Ltre/FmlySys/internal/asset"
	"github.com/Ltre/FmlySys/internal/store"
)

const moneyRecordHeader = "X-Fmly-Record-Key"

func setMoneyRecordHeader(w http.ResponseWriter, kind string, id int64) {
	if id > 0 {
		w.Header().Set(moneyRecordHeader, kind+":"+strconv.FormatInt(id, 10))
	}
}

// WithMoneyWorkflowV3 is layered outside the earlier asset workflow fixes. It
// keeps all existing URLs but makes the four money writes atomic and bounded.
func (s *Server) WithMoneyWorkflowV3(next http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /assets/self-events", s.member("assets.self_change", s.createSelfAssetEventV3))
	mux.HandleFunc("POST /assets/expenses", s.member("expenses.create", s.createExpenseV3))
	mux.HandleFunc("POST /assets/transfers", s.member("transfers.create", s.createTransferV3))
	mux.HandleFunc("POST /assets/reimbursements", s.member("reimbursements.create", s.createReimbursementV3))
	mux.HandleFunc("POST /admin/assets/events", s.adminOnly(s.adminCreateAssetEventV3))
	mux.HandleFunc("POST /admin/assets/expenses", s.adminOnly(s.adminCreateExpenseV3))
	mux.HandleFunc("POST /admin/assets/transfers", s.adminOnly(s.adminCreateTransferV3))
	mux.HandleFunc("POST /admin/assets/reimbursements", s.adminOnly(s.adminCreateReimbursementV3))
	mux.HandleFunc("GET /api/money-record/{kind}/{id}", s.memberOrAdmin("assets.view", s.moneyRecordInfo))
	mux.Handle("/", next)
	return mux
}

func (s *Server) createSelfAssetEventV3(w http.ResponseWriter, r *http.Request) {
	if _, err := parseWorkflowRequest(w, r, false); err != nil {
		s.fail(w, r, err)
		return
	}
	typ := r.FormValue("event_type")
	if typ != "ASSET_IN" && typ != "ASSET_OUT" {
		s.fail(w, r, fmt.Errorf("前台只允许登记资产新增或资产减少"))
		return
	}
	amount, err := asset.ParseYuan(r.FormValue("amount"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	actor := currentMember(r).ID
	id, err := s.Store.CreateAssetEventWorkflowAtomic(r.Context(), actor, actor, typ, amount, r.FormValue("description"), formDateTime(r.FormValue("occurred_at")), 0)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	setMoneyRecordHeader(w, "asset_event", id)
	redirect(w, r, "/assets")
}

func (s *Server) adminCreateAssetEventV3(w http.ResponseWriter, r *http.Request) {
	if _, err := parseWorkflowRequest(w, r, false); err != nil {
		s.fail(w, r, err)
		return
	}
	amount, err := asset.ParseYuan(r.FormValue("amount"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	id, err := s.Store.CreateAssetEventWorkflowAtomic(r.Context(), s.DevActorID, parseID(r.FormValue("holder_id")), r.FormValue("event_type"), amount, r.FormValue("description"), formDateTime(r.FormValue("occurred_at")), parseID(r.FormValue("related_event_id")))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	setMoneyRecordHeader(w, "asset_event", id)
	redirect(w, r, "/admin")
}

func (s *Server) createExpenseV3(w http.ResponseWriter, r *http.Request) {
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
	id, err := s.Store.CreateExpenseWorkflowAtomic(r.Context(), actor, expenseInputFromWorkflow(r, actor, amount), evidenceDir(s), files)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	setMoneyRecordHeader(w, "expense", id)
	redirect(w, r, "/assets")
}

func (s *Server) adminCreateExpenseV3(w http.ResponseWriter, r *http.Request) {
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
	id, err := s.Store.CreateExpenseWorkflowAtomic(r.Context(), s.DevActorID, in, evidenceDir(s), files)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	setMoneyRecordHeader(w, "expense", id)
	redirect(w, r, "/admin")
}

func transferParties(r *http.Request, actor int64) (int64, int64, error) {
	other := parseID(r.FormValue("counterparty_id"))
	switch r.FormValue("direction") {
	case "FROM":
		return other, actor, nil
	case "TO":
		return actor, other, nil
	default:
		return 0, 0, fmt.Errorf("请选择转账方向")
	}
}

func (s *Server) createTransferV3(w http.ResponseWriter, r *http.Request) {
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
	from, to, err := transferParties(r, actor)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	id, err := s.Store.CreateTransferWorkflowAtomic(r.Context(), actor, from, to, amount, r.FormValue("purpose"), r.FormValue("payment_channel"), formDateTime(r.FormValue("occurred_at")), parseID(r.FormValue("matter_id")), evidenceDir(s), files)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	setMoneyRecordHeader(w, "transfer", id)
	redirect(w, r, "/assets")
}

func (s *Server) adminCreateTransferV3(w http.ResponseWriter, r *http.Request) {
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
	id, err := s.Store.CreateTransferWorkflowAtomic(r.Context(), s.DevActorID, parseID(r.FormValue("from_id")), parseID(r.FormValue("to_id")), amount, r.FormValue("purpose"), r.FormValue("payment_channel"), formDateTime(r.FormValue("occurred_at")), parseID(r.FormValue("matter_id")), evidenceDir(s), files)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	setMoneyRecordHeader(w, "transfer", id)
	redirect(w, r, "/admin")
}

func (s *Server) createReimbursementV3(w http.ResponseWriter, r *http.Request) {
	s.createReimbursementV3ForHolder(w, r, currentMember(r).ID, currentMember(r).ID, "/assets")
}

func (s *Server) adminCreateReimbursementV3(w http.ResponseWriter, r *http.Request) {
	s.createReimbursementV3ForHolder(w, r, s.DevActorID, parseID(r.FormValue("holder_id")), "/admin")
}

func (s *Server) createReimbursementV3ForHolder(w http.ResponseWriter, r *http.Request, actor, holder int64, target string) {
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
	id, err := s.Store.CreateReimbursementWorkflowAtomic(r.Context(), actor, parseID(r.FormValue("expense_id")), holder, amount, r.FormValue("payment_channel"), formDateTime(r.FormValue("occurred_at")), r.FormValue("note"), evidenceDir(s), files)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	setMoneyRecordHeader(w, "reimbursement", id)
	redirect(w, r, target)
}

func recordDate(v string) string {
	if len(v) >= 10 {
		return v[:10]
	}
	return v
}

type moneyRecordResponse struct {
	OK      bool     `json:"ok"`
	Kind    string   `json:"kind"`
	ID      int64    `json:"id"`
	Section string   `json:"section"`
	Tokens  []string `json:"tokens,omitempty"`
	Href    string   `json:"href,omitempty"`
}

func appendToken(tokens []string, value string) []string {
	if strings.TrimSpace(value) != "" {
		tokens = append(tokens, value)
	}
	return tokens
}

func (s *Server) moneyRecordInfo(w http.ResponseWriter, r *http.Request) {
	id := parseID(r.PathValue("id"))
	record, err := s.Store.MoneyRecordByID(r.Context(), r.PathValue("kind"), id)
	if err != nil {
		passkeyJSONError(w, err, http.StatusNotFound)
		return
	}
	resp := moneyRecordResponse{OK: true, Kind: record.Kind, ID: id}
	switch record.Kind {
	case "expense":
		resp.Section = "expense-records"
		resp.Href = "/assets/expenses/" + strconv.FormatInt(id, 10) + "/edit"
	case "transfer":
		resp.Section = "transfer-records"
		resp.Tokens = []string{recordDate(record.OccurredAt), record.FromName, record.ToName, "¥" + asset.FormatYuan(record.AmountCent), record.PaymentChannel}
		resp.Tokens = appendToken(resp.Tokens, record.Purpose)
	case "reimbursement":
		resp.Section = "reimbursement-records"
		resp.Tokens = []string{recordDate(record.OccurredAt), record.ExpenseTitle, record.HolderName, record.ReceiverName, "¥" + asset.FormatYuan(record.AmountCent), record.PaymentChannel}
	case "asset_event":
		resp.Section = "asset-movements"
		resp.Tokens = []string{recordDate(record.OccurredAt), record.TypeLabel, record.HolderName}
		resp.Tokens = appendToken(resp.Tokens, record.Description)
	default:
		passkeyJSONError(w, fmt.Errorf("不支持的记录类型"), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(resp)
}
