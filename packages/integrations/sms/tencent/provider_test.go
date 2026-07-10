package tencent

import (
	"testing"

	smsint "github.com/imbytecat/moonbase/integrations/sms"
)

func TestRequestPreservesTencentProtocolShape(t *testing.T) {
	req := newRequest(t.Context(), providerConfig{SDKAppID: "app", SignName: "签名", TemplateID: "default"}, smsint.Message{E164: "+8613800138000", Content: "123456"})
	if len(req.PhoneNumberSet) != 1 || *req.PhoneNumberSet[0] != "+8613800138000" || *req.TemplateId != "default" || len(req.TemplateParamSet) != 1 || *req.TemplateParamSet[0] != "123456" {
		t.Fatalf("request = %+v", req)
	}
}
