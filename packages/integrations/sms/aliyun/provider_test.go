package aliyun

import (
	"testing"

	smsint "github.com/imbytecat/moonbase/integrations/sms"
)

func TestRequestValuesPreserveAliyunProtocolShape(t *testing.T) {
	target, template, params, err := requestValues(
		providerConfig{TemplateCode: "SMS_DEFAULT"},
		smsint.Message{E164: "+8613800138000", Content: "123456"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if target != "13800138000" || template != "SMS_DEFAULT" || params != `{"code":"123456"}` {
		t.Fatalf("got %q %q %q", target, template, params)
	}
	target, template, _, err = requestValues(
		providerConfig{TemplateCode: "SMS_DEFAULT"},
		smsint.Message{TemplateCode: "SMS_CUSTOM", E164: "+14155552671", Content: "hello"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if target != "+14155552671" || template != "SMS_CUSTOM" {
		t.Fatalf("got %q %q", target, template)
	}
}
