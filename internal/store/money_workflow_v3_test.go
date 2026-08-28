package store

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestMoneyWorkflowGateHonorsContext(t *testing.T) {
	moneyWorkflowGate <- struct{}{}
	defer func() { <-moneyWorkflowGate }()

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	_, err := acquireMoneyWorkflow(ctx)
	if err == nil {
		t.Fatal("expected busy gate to time out")
	}
	if !strings.Contains(err.Error(), "超时") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestQuickAssetTypeIsLimitedToSelfChangeTypes(t *testing.T) {
	for _, typ := range []string{"ASSET_IN", "ASSET_OUT"} {
		if err := validateQuickAssetType(typ); err != nil {
			t.Fatalf("%s should be accepted: %v", typ, err)
		}
	}
	for _, typ := range []string{"INITIAL_ASSET", "ADJUSTMENT", "EXPENSE_REIMBURSEMENT"} {
		if err := validateQuickAssetType(typ); err == nil {
			t.Fatalf("%s should be rejected", typ)
		}
	}
}
