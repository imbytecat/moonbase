// Package i18n localizes backend-originated text — outbound email/SMS and
// in-app notification content — that reaches a user with no client present to
// translate it. RPC error messages are deliberately NOT here: those stay
// stable error codes the frontend humanizes, so translation lives once, on the
// client. The catalog is code (like the permission catalog): add a key + both
// locales, never a per-message table.
package i18n

import (
	"fmt"

	"golang.org/x/text/language"
)

type Locale string

const (
	ZhCN    Locale = "zh-CN"
	En      Locale = "en"
	Default        = ZhCN
)

// Supported lists the locales the backend can render.
var Supported = []Locale{ZhCN, En}

// Resolve picks a recipient's language by precedence: their stored account
// preference wins, then the request's Accept-Language, then the default.
// userLocale is empty when the account never set one; acceptLanguage is the
// raw header (empty for out-of-band sends like a permission fan-out). Matching
// is by base language against the supported set — an unsupported language
// falls to the default rather than x/text's English-as-fallback heuristic.
func Resolve(userLocale, acceptLanguage string) Locale {
	if l, ok := normalize(userLocale); ok {
		return l
	}
	tags, _, err := language.ParseAcceptLanguage(acceptLanguage)
	if err == nil {
		for _, tag := range tags {
			base, _ := tag.Base()
			switch base.String() {
			case "zh":
				return ZhCN
			case "en":
				return En
			}
		}
	}
	return Default
}

func normalize(s string) (Locale, bool) {
	for _, l := range Supported {
		if string(l) == s {
			return l, true
		}
	}
	return "", false
}

const (
	AuthVerifyEmailSubject   = "auth.verifyEmail.subject"
	AuthVerifyEmailBody      = "auth.verifyEmail.body"
	AuthResetPasswordSubject = "auth.resetPassword.subject"
	AuthResetPasswordBody    = "auth.resetPassword.body"
	AuthCodeSubject          = "auth.code.subject"
	AuthCodeBody             = "auth.code.body"
	AuthCodeBodyNamed        = "auth.code.bodyNamed"

	NotifRoleChangedTitle = "notif.roleChanged.title"
	NotifRoleChangedBody  = "notif.roleChanged.body"
)

var catalog = map[Locale]map[string]string{
	ZhCN: {
		AuthVerifyEmailSubject:   "验证你的邮箱",
		AuthVerifyEmailBody:      "你好 %s，\n\n点击下方链接验证你的邮箱地址，链接 24 小时内有效。\n\n%s\n",
		AuthResetPasswordSubject: "重置你的密码",
		AuthResetPasswordBody:    "你好 %s，\n\n点击下方链接设置新密码，链接 1 小时内有效。如果这不是你本人操作，请忽略此邮件。\n\n%s\n",
		AuthCodeSubject:          "你的验证码",
		AuthCodeBody:             "你的验证码是 %s，5 分钟内有效。\n",
		AuthCodeBodyNamed:        "你好 %s，\n\n你的验证码是 %s，5 分钟内有效。\n",
		NotifRoleChangedTitle:    "你的角色已更新",
		NotifRoleChangedBody:     "管理员更新了你的账号角色，你的权限可能已变化。",
	},
	En: {
		AuthVerifyEmailSubject:   "Verify your email",
		AuthVerifyEmailBody:      "Hi %s,\n\nClick the link below to verify your email address. The link expires in 24 hours.\n\n%s\n",
		AuthResetPasswordSubject: "Reset your password",
		AuthResetPasswordBody:    "Hi %s,\n\nClick the link below to set a new password. The link expires in 1 hour. If you didn't request this, ignore this email.\n\n%s\n",
		AuthCodeSubject:          "Your verification code",
		AuthCodeBody:             "Your verification code is %s. It expires in 5 minutes.\n",
		AuthCodeBodyNamed:        "Hi %s,\n\nYour verification code is %s. It expires in 5 minutes.\n",
		NotifRoleChangedTitle:    "Your roles were updated",
		NotifRoleChangedBody:     "An administrator updated your account roles; your permissions may have changed.",
	},
}

// T renders a catalog message in the given locale, filling %-verb args. An
// unknown locale or key falls back to the default locale, then to the key
// itself, so a missing translation degrades visibly rather than panicking.
func T(loc Locale, key string, args ...any) string {
	msg, ok := catalog[loc][key]
	if !ok {
		msg, ok = catalog[Default][key]
		if !ok {
			return key
		}
	}
	if len(args) == 0 {
		return msg
	}
	return fmt.Sprintf(msg, args...)
}
