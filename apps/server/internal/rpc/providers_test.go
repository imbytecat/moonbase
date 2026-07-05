package rpc

import (
	"slices"
	"testing"

	"buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/imbytecat/moonbase/server/internal/captcha"
	paymentv1 "github.com/imbytecat/moonbase/server/internal/gen/payment/v1"
	systemv1 "github.com/imbytecat/moonbase/server/internal/gen/system/v1"
	"github.com/imbytecat/moonbase/server/internal/llm"
	"github.com/imbytecat/moonbase/server/internal/mail"
	"github.com/imbytecat/moonbase/server/internal/oauth"
	"github.com/imbytecat/moonbase/server/internal/pay"
	"github.com/imbytecat/moonbase/server/internal/sms"
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

// TestPaymentMethodsMatchContract keeps the per-order method catalog aligned
// with the proto `in:` constraint the same way providers are.
func TestPaymentMethodsMatchContract(t *testing.T) {
	desc := (&paymentv1.CreatePaymentOrderRequest{}).ProtoReflect().Descriptor()
	contract := stringInConstraint(t, desc, "method")
	if !slices.Equal(contract, pay.Methods()) {
		t.Fatalf("method mismatch:\n  proto contract: %v\n  pay.Methods(): %v", contract, pay.Methods())
	}
}

// TestPaymentProfileMethodsMatchContract keeps the signed-products field's
// allowed values aligned with the union of driver catalogs, the same way the
// per-order method is.
func TestPaymentProfileMethodsMatchContract(t *testing.T) {
	desc := (&systemv1.PaymentProfile{}).ProtoReflect().Descriptor()
	contract := repeatedStringInConstraint(t, desc, "methods")
	if !slices.Equal(contract, pay.Methods()) {
		t.Fatalf("profile methods mismatch:\n  proto contract: %v\n  pay.Methods(): %v", contract, pay.Methods())
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

// repeatedStringInConstraint extracts the `in:` values of a repeated string
// field's per-item rule (buf.validate repeated.items.string.in).
func repeatedStringInConstraint(t *testing.T, desc protoreflect.MessageDescriptor, name protoreflect.Name) []string {
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
	if !ok || rules.GetRepeated().GetItems().GetString().GetIn() == nil {
		t.Fatalf("%s %s field has no repeated string in: rule", desc.FullName(), name)
	}
	out := []string{}
	for _, v := range rules.GetRepeated().GetItems().GetString().GetIn() {
		if v != "" {
			out = append(out, v)
		}
	}
	slices.Sort(out)
	return out
}
