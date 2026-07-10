// Package smtp sends email through a standard SMTP server.
package smtp

import (
	"context"
	"fmt"

	invopop "github.com/invopop/jsonschema"
	gomail "github.com/wneessen/go-mail"

	"github.com/imbytecat/moonbase/integrations/core/config"
	"github.com/imbytecat/moonbase/integrations/core/integration"
	"github.com/imbytecat/moonbase/integrations/email"
)

type encryption string

const (
	encryptionStartTLS encryption = "starttls"
	encryptionSSL      encryption = "ssl"
	encryptionNone     encryption = "none"
)

func (encryption) JSONSchema() *invopop.Schema {
	return &invopop.Schema{
		Type: "string",
		OneOf: []*invopop.Schema{
			{Const: string(encryptionStartTLS), Title: "STARTTLS", Description: "常用 587 端口，连接后升级为加密"},
			{Const: string(encryptionSSL), Title: "SSL/TLS", Description: "常用 465 端口，全程加密"},
			{Const: string(encryptionNone), Title: "不加密", Description: "明文传输，仅用于本地调试"},
		},
	}
}

type providerConfig struct {
	FromAddress string     `json:"fromAddress" jsonschema:"required,title=发件地址,minLength=1,maxLength=254"`
	FromName    string     `json:"fromName,omitempty" jsonschema:"title=发件人名称,maxLength=100"`
	Host        string     `json:"host" jsonschema:"required,title=服务器地址,minLength=1,maxLength=253"`
	Port        int        `json:"port" jsonschema:"required,title=端口,default=587,minimum=1,maximum=65535"`
	Username    string     `json:"username,omitempty" jsonschema:"title=用户名,maxLength=128"`
	Password    string     `json:"password,omitempty" jsonschema:"title=密码,minLength=1,maxLength=128"`
	Encryption  encryption `json:"encryption" jsonschema:"required,title=加密方式,default=starttls"`
}

func (providerConfig) JSONSchemaExtend(schema *invopop.Schema) {
	schema.DependentRequired = map[string][]string{
		"username": {"password"},
		"password": {"username"},
	}
}

type driver struct{}

func New() email.Registration {
	return email.Register(
		"smtp",
		integration.Presentation{
			Name:        "SMTP 邮件",
			Description: "通过标准 SMTP 服务器发送邮件",
			Color:       "#1677ff",
			IconRef:     "antd:MailOutlined",
		},
		config.MustContract[providerConfig](config.Policy{Secrets: []string{"/password"}}),
		driver{}.send,
	)
}

func (driver) send(ctx context.Context, cfg providerConfig, message email.Message) error {
	mail := gomail.NewMsg()
	if cfg.FromName != "" {
		if err := mail.FromFormat(cfg.FromName, cfg.FromAddress); err != nil {
			return fmt.Errorf("invalid from address: %w", err)
		}
	} else if err := mail.From(cfg.FromAddress); err != nil {
		return fmt.Errorf("invalid from address: %w", err)
	}
	if err := mail.To(message.To); err != nil {
		return fmt.Errorf("invalid recipient: %w", err)
	}
	mail.Subject(message.Subject)
	mail.SetBodyString(gomail.TypeTextPlain, message.TextBody)

	options := []gomail.Option{gomail.WithPort(cfg.Port)}
	switch cfg.Encryption {
	case encryptionSSL:
		options = append(options, gomail.WithSSLPort(false))
	case encryptionNone:
		options = append(options, gomail.WithTLSPolicy(gomail.NoTLS))
	default:
		options = append(options, gomail.WithTLSPolicy(gomail.TLSMandatory))
	}
	if cfg.Username != "" {
		options = append(options,
			gomail.WithSMTPAuth(gomail.SMTPAuthAutoDiscover),
			gomail.WithUsername(cfg.Username),
			gomail.WithPassword(cfg.Password),
		)
	}

	client, err := gomail.NewClient(cfg.Host, options...)
	if err != nil {
		return fmt.Errorf("create smtp client: %w", err)
	}
	if err := client.DialAndSendWithContext(ctx, mail); err != nil {
		return fmt.Errorf("send mail: %w", err)
	}
	return nil
}
