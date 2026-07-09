package rpc

import (
	"slices"
	"testing"

	"buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/imbytecat/moonbase/server/integrations/captcha"
	mail "github.com/imbytecat/moonbase/server/integrations/email"
	"github.com/imbytecat/moonbase/server/integrations/llm"
	"github.com/imbytecat/moonbase/server/integrations/oauth"
	"github.com/imbytecat/moonbase/server/integrations/sms"
	systemv1 "github.com/imbytecat/moonbase/server/internal/gen/system/v1"
	"github.com/imbytecat/moonbase/server/internal/pay"
	"github.com/imbytecat/moonbase/server/internal/storage"
)

// TestProviderRegistriesMatchContract walks the proto descriptors and asserts
// every provider value the wire contract accepts has a registered driver, and
// vice versa. Adding a provider to only one side (proto `in:` list or the Go
// registry) fails here instead of becoming a silently dead option.
func TestProviderRegistriesMatchContract(t *testing.T) {
	cases := []struct {
		message    proto.Message
		registered []string
	}{
		{&systemv1.StorageProfile{}, storage.Providers()},
		{&systemv1.EmailProfile{}, mail.Providers()},
		{&systemv1.SmsProfile{}, sms.Providers()},
		{&systemv1.CaptchaProfile{}, captcha.Providers()},
		{&systemv1.LlmProfile{}, llm.Providers()},
		{&systemv1.OauthProfile{}, oauth.Providers()},
		{&systemv1.PaymentProfile{}, pay.Providers()},
	}
	for _, tc := range cases {
		desc := tc.message.ProtoReflect().Descriptor()
		t.Run(string(desc.Name()), func(t *testing.T) {
			contract := providerConstraint(t, desc)
			if !slices.Equal(contract, tc.registered) {
				t.Fatalf("provider mismatch:\n  proto contract: %v\n  registered drivers: %v",
					contract, tc.registered)
			}
		})
	}
}

// providerConstraint extracts the non-empty values of the `in:` rule on the
// message's `provider` field.
func providerConstraint(t *testing.T, desc protoreflect.MessageDescriptor) []string {
	t.Helper()
	return stringInConstraint(t, desc, "provider")
}

func stringInConstraint(t *testing.T, desc protoreflect.MessageDescriptor, name protoreflect.Name) []string {
	t.Helper()
	field := desc.Fields().ByName(name)
	if field == nil {
		t.Fatalf("%s has no %s field", desc.FullName(), name)
	}
	opts, ok := field.Options().(*descriptorpb.FieldOptions)
	if !ok {
		t.Fatalf("%s %s field has unexpected options type", desc.FullName(), name)
	}
	rules, ok := proto.GetExtension(opts, validate.E_Field).(*validate.FieldRules)
	if !ok || rules.GetString().GetIn() == nil {
		t.Fatalf("%s %s field has no string in: rule", desc.FullName(), name)
	}
	out := []string{}
	for _, v := range rules.GetString().GetIn() {
		if v != "" {
			out = append(out, v)
		}
	}
	slices.Sort(out)
	return out
}
