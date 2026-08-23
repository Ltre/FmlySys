package httpserver

import (
	"net/http"

	"github.com/Ltre/FmlySys/internal/store"
)

type moneyRecordDetailView struct {
	Title           string
	ActivePartition string
	CurrentMember   store.Member
	Permissions     map[string]bool
	AdminUsername   string
	Record          store.MoneyRecordLocator
	KindLabel       string
}

func moneyRecordKindLabel(kind string) string {
	switch kind {
	case "asset_event":
		return "资产变动"
	case "expense":
		return "公共消费"
	case "transfer":
		return "内部转账"
	case "reimbursement":
		return "报销"
	default:
		return kind
	}
}

func (s *Server) WithMoneyRecordDetails(next http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /money-record/{kind}/{id}", s.memberOrAdmin("assets.view", s.moneyRecordDetail))
	mux.Handle("/", next)
	return mux
}

func (s *Server) moneyRecordDetail(w http.ResponseWriter, r *http.Request) {
	record, err := s.Store.MoneyRecordByID(r.Context(), r.PathValue("kind"), parseID(r.PathValue("id")))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	v := moneyRecordDetailView{
		Title:           "资金记录详情",
		ActivePartition: s.PM.ActiveID,
		CurrentMember:   currentMember(r),
		Permissions:     currentPermissions(r),
		AdminUsername:   currentAdmin(r).Username,
		Record:          record,
		KindLabel:       moneyRecordKindLabel(record.Kind),
	}
	if err := s.Templates.ExecuteTemplate(w, "money-record-detail.html", v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
