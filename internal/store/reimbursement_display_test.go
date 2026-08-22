package store

import "testing"

func TestExpenseReimbursementBreakdown(t *testing.T) {
	e := Expense{
		AmountCent:       900000,
		ReimbursableCent: 800000,
		ReimbursedCent:   250000,
		PendingCent:      550000,
	}
	if got := e.AutoReimbursedCent(); got != 100000 {
		t.Fatalf("AutoReimbursedCent=%d, want 100000", got)
	}
	if got := e.ManualReimbursedCent(); got != 250000 {
		t.Fatalf("ManualReimbursedCent=%d, want 250000", got)
	}
	if got := e.TotalReimbursedCent(); got != 350000 {
		t.Fatalf("TotalReimbursedCent=%d, want 350000", got)
	}
}

func TestAssetEventHumanReadableTypeAndDelta(t *testing.T) {
	cases := []struct {
		event AssetEvent
		label string
		delta int64
		sign  string
	}{
		{AssetEvent{Type: "INITIAL_ASSET", AmountCent: 1000}, "初始资产", 1000, "+"},
		{AssetEvent{Type: "ASSET_IN", AmountCent: 2000}, "资产新增", 2000, "+"},
		{AssetEvent{Type: "ASSET_OUT", AmountCent: 3000}, "资产减少", -3000, "−"},
		{AssetEvent{Type: "ADJUSTMENT", AmountCent: -4000}, "财务调整", -4000, "−"},
		{AssetEvent{Type: "EXPENSE_REIMBURSEMENT", AmountCent: -5000}, "消费报销", -5000, "−"},
	}
	for _, tc := range cases {
		if got := tc.event.TypeLabel(); got != tc.label {
			t.Fatalf("%s label=%q, want %q", tc.event.Type, got, tc.label)
		}
		if got := tc.event.BalanceDeltaCent(); got != tc.delta {
			t.Fatalf("%s delta=%d, want %d", tc.event.Type, got, tc.delta)
		}
		if got := tc.event.BalanceSign(); got != tc.sign {
			t.Fatalf("%s sign=%q, want %q", tc.event.Type, got, tc.sign)
		}
	}
}
