package alipay

import (
	"testing"
	"time"

	pay "github.com/imbytecat/moonbase/integrations/payment"
	"github.com/smartwalle/alipay/v3"
)

func TestStateMapping(t *testing.T) {
	cases := []struct {
		in   alipay.TradeStatus
		want pay.State
	}{
		{alipay.TradeStatusSuccess, pay.StatePaid},
		{alipay.TradeStatusFinished, pay.StatePaid},
		{alipay.TradeStatusClosed, pay.StateClosed},
		{alipay.TradeStatusWaitBuyerPay, pay.StatePending},
		{"", pay.StatePending},
	}
	for _, test := range cases {
		if got := alipayState(test.in); got != test.want {
			t.Errorf("alipayState(%q) = %v, want %v", test.in, got, test.want)
		}
	}
}

func TestTimeParsesBeijingTime(t *testing.T) {
	got := alipayTime("2026-07-11 12:30:00")
	want := time.Date(2026, 7, 11, 12, 30, 0, 0, time.FixedZone("CST", 8*3600))
	if !got.Equal(want) {
		t.Errorf("alipayTime = %v, want %v", got, want)
	}
	if !alipayTime("").IsZero() || !alipayTime("garbage").IsZero() {
		t.Error("empty or malformed timestamp should parse to zero time")
	}
}
