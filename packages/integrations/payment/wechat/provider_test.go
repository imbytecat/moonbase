package wechat

import (
	"testing"

	pay "github.com/imbytecat/moonbase/integrations/payment"
)

func TestTradeStateMapping(t *testing.T) {
	cases := []struct {
		in   string
		want pay.State
	}{
		{"SUCCESS", pay.StatePaid},
		{"CLOSED", pay.StateClosed},
		{"REVOKED", pay.StateClosed},
		{"PAYERROR", pay.StateClosed},
		{"REFUND", pay.StateRefunded},
		{"NOTPAY", pay.StatePending},
		{"USERPAYING", pay.StatePending},
	}
	for _, test := range cases {
		if got := wechatTradeState(test.in); got != test.want {
			t.Errorf("wechatTradeState(%q) = %v, want %v", test.in, got, test.want)
		}
	}
}
