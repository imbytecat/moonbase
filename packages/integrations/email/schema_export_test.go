package email_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/imbytecat/moonbase/integrations/email"
	"github.com/imbytecat/moonbase/integrations/email/cloudflare"
	"github.com/imbytecat/moonbase/integrations/email/smtp"
)

func TestExportProviderSchemasForWeb(t *testing.T) {
	output := os.Getenv("MOONBASE_PROVIDER_SCHEMA_OUTPUT")
	if output == "" {
		t.Skip("only used by the Web cross-runtime contract test")
	}
	registry := email.MustRegistry(smtp.New(), cloudflare.New(nil))
	raw, err := json.Marshal(registry.Descriptors())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(output, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}
