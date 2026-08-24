package store

type Member struct {
	ID             int64
	Name, Relation string
}
type AssetSummary struct {
	NetCent, HolderTotalCent, PendingCent, DifferenceCent int64
	Holders                                               []HolderBalance
}
type HolderBalance struct {
	MemberID int64
	Name     string
	Cent     int64
}
type AssetEvent struct {
	ID                                  int64
	Type                                string
	AmountCent                          int64
	HolderName, Description, OccurredAt string
	RelatedEventID                      int64
	RelatedLabel                        string
}
type AssetInflowOption struct {
	ID                                        int64
	HolderID                                  int64
	HolderName, Type, Description, OccurredAt string
	AmountCent                                int64
}
type Transfer struct {
	ID                                               int64
	FromName, ToName                                 string
	AmountCent                                       int64
	Purpose, PaymentChannel, OccurredAt, MatterTitle string
	Evidence                                         []Evidence
}
type Reimbursement struct {
	ID                                     int64
	ExpenseID                              int64
	ExpenseTitle, HolderName, ReceiverName string
	AmountCent                             int64
	PaymentChannel, OccurredAt, Note       string
	Evidence                               []Evidence
}
type Expense struct {
	ID                                                 int64
	Title, Category                                    string
	AmountCent                                         int64
	OccurredAt, FundingType, PayerName, HolderName     string
	ReimbursableCent, ReimbursedCent, PendingCent      int64
	Description, PaymentChannel, Merchant, MatterTitle string
	Evidence                                           []Evidence
}

func (e Expense) AutoReimbursedCent() int64 {
	v := e.AmountCent - e.ReimbursableCent
	if v < 0 {
		return 0
	}
	return v
}

func (e Expense) ManualReimbursedCent() int64 {
	if e.ReimbursedCent < 0 {
		return 0
	}
	return e.ReimbursedCent
}

func (e Expense) TotalReimbursedCent() int64 {
	return e.AutoReimbursedCent() + e.ManualReimbursedCent()
}

func (e AssetEvent) TypeLabel() string {
	switch e.Type {
	case "INITIAL_ASSET":
		return "初始资产"
	case "ASSET_IN":
		return "资产新增"
	case "ASSET_OUT":
		return "资产减少"
	case "ADJUSTMENT":
		return "财务调整"
	case "EXPENSE_REIMBURSEMENT":
		return "消费报销"
	default:
		return e.Type
	}
}

func (e AssetEvent) BalanceDeltaCent() int64 {
	if e.Type == "ASSET_OUT" {
		if e.AmountCent < 0 {
			return e.AmountCent
		}
		return -e.AmountCent
	}
	return e.AmountCent
}

func (e AssetEvent) BalanceAmountCent() int64 {
	v := e.BalanceDeltaCent()
	if v < 0 {
		return -v
	}
	return v
}

func (e AssetEvent) BalanceSign() string {
	v := e.BalanceDeltaCent()
	if v < 0 {
		return "−"
	}
	if v > 0 {
		return "+"
	}
	return ""
}

type AuditLog struct {
	ID                                                          int64
	ActorName, Action, BeforeJSON, AfterJSON, Reason, CreatedAt string
}
type Evidence struct {
	ID                     int64
	EntityType             string
	EntityID               int64
	OriginalName, MimeType string
	Size                   int64
}
type Matter struct {
	ID                                                                           int64
	ParentID                                                                     *int64
	ParentIDValue, OwnerMemberID                                                 int64
	ParentTitle, Title, Type, Description, Status, StartDate, DueDate, OwnerName string
	ExpenseCent                                                                  int64
}
type Archive struct {
	ID                                              int64
	Title, Category, Content, Visibility, CreatedAt string
	Attachments                                     []Attachment
}
type Attachment struct {
	ID                     int64
	OriginalName, MimeType string
	Size                   int64
}
