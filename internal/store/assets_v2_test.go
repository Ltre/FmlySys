package store

import "testing"

func TestValidatePaymentChannel(t *testing.T) {
	for _, v := range []string{"支付宝", "微信", "银行", "现金", "其它"} {
		if err := validatePaymentChannel(v); err != nil {
			t.Fatalf("%s should be valid: %v", v, err)
		}
	}
	if err := validatePaymentChannel("银行卡"); err == nil {
		t.Fatal("unexpected custom channel accepted")
	}
}
