package store

type Member struct {
	ID       int64
	Name     string
	Relation string
}

type AssetSummary struct {
	NetCent         int64
	HolderTotalCent int64
	PendingCent     int64
	DifferenceCent  int64
	Holders         []HolderBalance
}

type HolderBalance struct {
	MemberID int64
	Name     string
	Cent     int64
}

type AssetEvent struct {
	ID          int64
	Type        string
	AmountCent  int64
	HolderName  string
	Description string
	OccurredAt  string
}

type Transfer struct {
	ID             int64
	FromName       string
	ToName         string
	AmountCent     int64
	Purpose        string
	PaymentChannel string
	OccurredAt     string
	MatterTitle    string
}

type Reimbursement struct {
	ID             int64
	ExpenseTitle   string
	HolderName     string
	ReceiverName   string
	AmountCent     int64
	PaymentChannel string
	OccurredAt     string
}

type Expense struct {
	ID               int64
	Title            string
	Category         string
	AmountCent       int64
	OccurredAt       string
	FundingType      string
	PayerName        string
	HolderName       string
	ReimbursableCent int64
	ReimbursedCent   int64
	PendingCent      int64
	Description      string
	PaymentChannel   string
	Merchant         string
	MatterTitle      string
}

type Matter struct {
	ID          int64
	ParentID    *int64
	ParentTitle string
	Title       string
	Type        string
	Description string
	Status      string
	StartDate   string
	DueDate     string
	OwnerName   string
	ExpenseCent int64
}

type Archive struct {
	ID          int64
	Title       string
	Category    string
	Content     string
	Visibility  string
	CreatedAt   string
	Attachments []Attachment
}

type Attachment struct {
	ID           int64
	OriginalName string
	MimeType     string
	Size         int64
}
