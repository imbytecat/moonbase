package wechat

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	oauthint "github.com/imbytecat/moonbase/integrations/oauth"
)

// WeChat Open Platform website-application QR login. The oddities drivers
// exist to absorb: appid/secret naming, errcode-in-200-body errors, unionid
// (stable across apps of one Open Platform account) preferred over openid,
// and the #wechat_redirect fragment the authorize URL requires.
const (
	wechatAuthorizeEndpoint = "https://open.weixin.qq.com/connect/qrconnect"
	wechatTokenEndpoint     = "https://api.weixin.qq.com/sns/oauth2/access_token"
	wechatUserInfoEndpoint  = "https://api.weixin.qq.com/sns/userinfo"
)

var wechatHTTP = &http.Client{Timeout: 10 * time.Second}

func wechatAuthorizeURL(_ context.Context, config providerConfig, redirectURI, state string) (string, oauthint.FlowSecrets, error) {
	url := wechatAuthorizeEndpoint + "?" + encodeQuery(
		"appid", config.AppID,
		"redirect_uri", redirectURI,
		"response_type", "code",
		"scope", "snsapi_login",
		"state", state,
	) + "#wechat_redirect"
	return url, oauthint.FlowSecrets{}, nil
}

type wechatTokenResponse struct {
	AccessToken string `json:"access_token"`
	OpenID      string `json:"openid"`
	UnionID     string `json:"unionid"`
	ErrCode     int    `json:"errcode"`
	ErrMsg      string `json:"errmsg"`
}

type wechatUserInfoResponse struct {
	Nickname   string `json:"nickname"`
	HeadImgURL string `json:"headimgurl"`
	UnionID    string `json:"unionid"`
	ErrCode    int    `json:"errcode"`
	ErrMsg     string `json:"errmsg"`
}

func wechatExchange(ctx context.Context, config providerConfig, code, _ string, _ oauthint.FlowSecrets) (oauthint.ExternalIdentity, error) {
	tokenURL := wechatTokenEndpoint + "?" + encodeQuery(
		"appid", config.AppID,
		"secret", config.AppSecret,
		"code", code,
		"grant_type", "authorization_code",
	)
	var token wechatTokenResponse
	if err := wechatGet(ctx, tokenURL, &token); err != nil {
		return oauthint.ExternalIdentity{}, fmt.Errorf("wechat token exchange: %w", err)
	}
	if token.ErrCode != 0 {
		return oauthint.ExternalIdentity{}, fmt.Errorf("wechat token exchange: %s (%d)", token.ErrMsg, token.ErrCode)
	}

	infoURL := wechatUserInfoEndpoint + "?" + encodeQuery(
		"access_token", token.AccessToken,
		"openid", token.OpenID,
	)
	var info wechatUserInfoResponse
	if err := wechatGet(ctx, infoURL, &info); err != nil {
		return oauthint.ExternalIdentity{}, fmt.Errorf("wechat userinfo: %w", err)
	}
	if info.ErrCode != 0 {
		return oauthint.ExternalIdentity{}, fmt.Errorf("wechat userinfo: %s (%d)", info.ErrMsg, info.ErrCode)
	}

	subject := token.UnionID
	if subject == "" {
		subject = info.UnionID
	}
	if subject == "" {
		subject = token.OpenID
	}
	if subject == "" {
		return oauthint.ExternalIdentity{}, fmt.Errorf("wechat userinfo: no unionid or openid")
	}
	return oauthint.ExternalIdentity{
		ProviderID: subject,
		Name:       info.Nickname,
		AvatarURL:  info.HeadImgURL,
	}, nil
}

func encodeQuery(pairs ...string) string {
	query := url.Values{}
	for i := 0; i+1 < len(pairs); i += 2 {
		query.Set(pairs[i], pairs[i+1])
	}
	return query.Encode()
}

func wechatGet(ctx context.Context, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := wechatHTTP.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	return json.Unmarshal(body, out)
}
