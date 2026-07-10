// Package cloudflare sends email through the Cloudflare Email Service API.
package cloudflare

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/imbytecat/moonbase/integrations/core/config"
	"github.com/imbytecat/moonbase/integrations/core/integration"
	"github.com/imbytecat/moonbase/integrations/email"
)

type providerConfig struct {
	FromAddress string `json:"fromAddress" jsonschema:"required,title=发件地址,minLength=1,maxLength=254"`
	FromName    string `json:"fromName,omitempty" jsonschema:"title=发件人名称,maxLength=100"`
	AccountID   string `json:"accountId" jsonschema:"required,title=账户 ID,minLength=1,maxLength=64"`
	APIToken    string `json:"apiToken" jsonschema:"required,title=API 令牌,minLength=1,maxLength=256"`
}

type driver struct{ http *http.Client }

func New(httpClient *http.Client) email.Registration {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return email.Register(
		"cloudflare",
		integration.Presentation{
			Name:        "Cloudflare 邮件",
			Description: "通过托管邮件发送接口投递邮件",
			Color:       "#f6821f",
			IconRef:     "antd:CloudOutlined",
		},
		config.MustContract[providerConfig](config.Policy{Secrets: []string{"/apiToken"}}),
		driver{http: httpClient}.send,
	)
}

func (d driver) send(ctx context.Context, cfg providerConfig, message email.Message) error {
	from := cfg.FromAddress
	if cfg.FromName != "" {
		from = fmt.Sprintf("%s <%s>", cfg.FromName, cfg.FromAddress)
	}
	payload, err := json.Marshal(map[string]string{
		"to":      message.To,
		"from":    from,
		"subject": message.Subject,
		"text":    message.TextBody,
	})
	if err != nil {
		return fmt.Errorf("encode cloudflare request: %w", err)
	}

	endpoint := fmt.Sprintf(
		"https://api.cloudflare.com/client/v4/accounts/%s/email/sending/send", cfg.AccountID)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+cfg.APIToken)
	request.Header.Set("Content-Type", "application/json")

	response, err := d.http.Do(request)
	if err != nil {
		return fmt.Errorf("cloudflare send: %w", err)
	}
	defer func() { _ = response.Body.Close() }()

	var result struct {
		Success bool `json:"success"`
		Errors  []struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"errors"`
	}
	body, _ := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("cloudflare send: http %d: %s", response.StatusCode, string(body))
	}
	if result.Success {
		return nil
	}
	if len(result.Errors) > 0 {
		return fmt.Errorf("cloudflare send: %s (code %d)", result.Errors[0].Message, result.Errors[0].Code)
	}
	return fmt.Errorf("cloudflare send: http %d", response.StatusCode)
}
