package rpc

import (
	"strings"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	systemv1 "github.com/imbytecat/moonbase/server/internal/gen/system/v1"
)

// TestKeepSecretsPreservesEveryMaskedField is the guardrail behind every
// integration's keepSecrets: a secret field is DETECTED behaviorally (set on
// create, masked out of the response), then an update carrying an empty
// value for it must keep the stored value. Adding a secret field to a proto
// profile without wiring its keepSecrets branch silently wipes stored
// credentials — this test turns that mistake red.
func TestKeepSecretsPreservesEveryMaskedField(t *testing.T) {
	cases := []struct {
		name    string
		profile func() proto.Message
		create  func(t *testing.T, svc *SystemService, p proto.Message) proto.Message
		update  func(t *testing.T, svc *SystemService, p proto.Message)
	}{
		{
			name:    "storage",
			profile: func() proto.Message { return &systemv1.StorageProfile{} },
			create: func(t *testing.T, svc *SystemService, p proto.Message) proto.Message {
				t.Helper()
				resp, err := svc.CreateStorageProfile(t.Context(), connect.NewRequest(
					&systemv1.CreateStorageProfileRequest{Profile: p.(*systemv1.StorageProfile)}))
				if err != nil {
					t.Fatal(err)
				}
				return resp.Msg.GetProfile()
			},
			update: func(t *testing.T, svc *SystemService, p proto.Message) {
				t.Helper()
				if _, err := svc.UpdateStorageProfile(t.Context(), connect.NewRequest(
					&systemv1.UpdateStorageProfileRequest{Profile: p.(*systemv1.StorageProfile)})); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name:    "captcha",
			profile: func() proto.Message { return &systemv1.CaptchaProfile{} },
			create: func(t *testing.T, svc *SystemService, p proto.Message) proto.Message {
				t.Helper()
				resp, err := svc.CreateCaptchaProfile(t.Context(), connect.NewRequest(
					&systemv1.CreateCaptchaProfileRequest{Profile: p.(*systemv1.CaptchaProfile)}))
				if err != nil {
					t.Fatal(err)
				}
				return resp.Msg.GetProfile()
			},
			update: func(t *testing.T, svc *SystemService, p proto.Message) {
				t.Helper()
				if _, err := svc.UpdateCaptchaProfile(t.Context(), connect.NewRequest(
					&systemv1.UpdateCaptchaProfileRequest{Profile: p.(*systemv1.CaptchaProfile)})); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name:    "email",
			profile: func() proto.Message { return &systemv1.EmailProfile{} },
			create: func(t *testing.T, svc *SystemService, p proto.Message) proto.Message {
				t.Helper()
				resp, err := svc.CreateEmailProfile(t.Context(), connect.NewRequest(
					&systemv1.CreateEmailProfileRequest{Profile: p.(*systemv1.EmailProfile)}))
				if err != nil {
					t.Fatal(err)
				}
				return resp.Msg.GetProfile()
			},
			update: func(t *testing.T, svc *SystemService, p proto.Message) {
				t.Helper()
				if _, err := svc.UpdateEmailProfile(t.Context(), connect.NewRequest(
					&systemv1.UpdateEmailProfileRequest{Profile: p.(*systemv1.EmailProfile)})); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name:    "sms",
			profile: func() proto.Message { return &systemv1.SmsProfile{} },
			create: func(t *testing.T, svc *SystemService, p proto.Message) proto.Message {
				t.Helper()
				resp, err := svc.CreateSmsProfile(t.Context(), connect.NewRequest(
					&systemv1.CreateSmsProfileRequest{Profile: p.(*systemv1.SmsProfile)}))
				if err != nil {
					t.Fatal(err)
				}
				return resp.Msg.GetProfile()
			},
			update: func(t *testing.T, svc *SystemService, p proto.Message) {
				t.Helper()
				if _, err := svc.UpdateSmsProfile(t.Context(), connect.NewRequest(
					&systemv1.UpdateSmsProfileRequest{Profile: p.(*systemv1.SmsProfile)})); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name:    "llm",
			profile: func() proto.Message { return &systemv1.LlmProfile{} },
			create: func(t *testing.T, svc *SystemService, p proto.Message) proto.Message {
				t.Helper()
				resp, err := svc.CreateLlmProfile(t.Context(), connect.NewRequest(
					&systemv1.CreateLlmProfileRequest{Profile: p.(*systemv1.LlmProfile)}))
				if err != nil {
					t.Fatal(err)
				}
				return resp.Msg.GetProfile()
			},
			update: func(t *testing.T, svc *SystemService, p proto.Message) {
				t.Helper()
				if _, err := svc.UpdateLlmProfile(t.Context(), connect.NewRequest(
					&systemv1.UpdateLlmProfileRequest{Profile: p.(*systemv1.LlmProfile)})); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name:    "oauth",
			profile: func() proto.Message { return &systemv1.OauthProfile{} },
			create: func(t *testing.T, svc *SystemService, p proto.Message) proto.Message {
				t.Helper()
				resp, err := svc.CreateOauthProfile(t.Context(), connect.NewRequest(
					&systemv1.CreateOauthProfileRequest{Profile: p.(*systemv1.OauthProfile)}))
				if err != nil {
					t.Fatal(err)
				}
				return resp.Msg.GetProfile()
			},
			update: func(t *testing.T, svc *SystemService, p proto.Message) {
				t.Helper()
				if _, err := svc.UpdateOauthProfile(t.Context(), connect.NewRequest(
					&systemv1.UpdateOauthProfileRequest{Profile: p.(*systemv1.OauthProfile)})); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name:    "payment",
			profile: func() proto.Message { return &systemv1.PaymentProfile{} },
			create: func(t *testing.T, svc *SystemService, p proto.Message) proto.Message {
				t.Helper()
				resp, err := svc.CreatePaymentProfile(t.Context(), connect.NewRequest(
					&systemv1.CreatePaymentProfileRequest{Profile: p.(*systemv1.PaymentProfile)}))
				if err != nil {
					t.Fatal(err)
				}
				return resp.Msg.GetProfile()
			},
			update: func(t *testing.T, svc *SystemService, p proto.Message) {
				t.Helper()
				if _, err := svc.UpdatePaymentProfile(t.Context(), connect.NewRequest(
					&systemv1.UpdatePaymentProfileRequest{Profile: p.(*systemv1.PaymentProfile)})); err != nil {
					t.Fatal(err)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q := newMemSettingsQuerier()
			svc, _ := newSystemService(q)

			in := tc.profile()
			fillStringFields(in.ProtoReflect(), "", func(path string) string {
				return "created|" + path
			})
			masked := tc.create(t, svc, in)

			inValues := map[string]string{}
			readStringFields(in.ProtoReflect(), "", inValues)
			maskedValues := map[string]string{}
			readStringFields(masked.ProtoReflect(), "", maskedValues)

			secrets := map[string]string{}
			for path, sentinel := range inValues {
				if path == "id" {
					continue
				}
				if maskedValues[path] != sentinel {
					secrets[path] = sentinel
				}
			}
			if len(secrets) == 0 {
				t.Fatalf("no write-only secret fields detected on %s — mask logic broken?", tc.name)
			}
			for path, sentinel := range secrets {
				if !storedContains(q, sentinel) {
					t.Fatalf("secret %s never reached the settings store — missing fromProto mapping?", path)
				}
			}

			upd := tc.profile()
			fillStringFields(upd.ProtoReflect(), "", func(path string) string {
				if _, isSecret := secrets[path]; isSecret {
					return ""
				}
				return "updated|" + path
			})
			idField := upd.ProtoReflect().Descriptor().Fields().ByName("id")
			createdID := masked.ProtoReflect().Get(idField).String()
			upd.ProtoReflect().Set(idField, protoreflect.ValueOfString(createdID))
			tc.update(t, svc, upd)

			for path, sentinel := range secrets {
				if !storedContains(q, sentinel) {
					t.Errorf("empty %s on update wiped the stored secret — add its keepSecrets branch", path)
				}
			}
		})
	}
}

func fillStringFields(m protoreflect.Message, prefix string, value func(path string) string) {
	fields := m.Descriptor().Fields()
	for i := range fields.Len() {
		fd := fields.Get(i)
		if fd.IsList() || fd.IsMap() {
			continue
		}
		switch fd.Kind() {
		case protoreflect.StringKind:
			if v := value(prefix + string(fd.Name())); v != "" {
				m.Set(fd, protoreflect.ValueOfString(v))
			}
		case protoreflect.MessageKind:
			fillStringFields(m.Mutable(fd).Message(), prefix+string(fd.Name())+".", value)
		default:
		}
	}
}

func readStringFields(m protoreflect.Message, prefix string, out map[string]string) {
	fields := m.Descriptor().Fields()
	for i := range fields.Len() {
		fd := fields.Get(i)
		if fd.IsList() || fd.IsMap() {
			continue
		}
		switch fd.Kind() {
		case protoreflect.StringKind:
			out[prefix+string(fd.Name())] = m.Get(fd).String()
		case protoreflect.MessageKind:
			readStringFields(m.Get(fd).Message(), prefix+string(fd.Name())+".", out)
		default:
		}
	}
}

func storedContains(q *memSettingsQuerier, sentinel string) bool {
	for _, raw := range q.rows {
		if strings.Contains(string(raw), sentinel) {
			return true
		}
	}
	return false
}
