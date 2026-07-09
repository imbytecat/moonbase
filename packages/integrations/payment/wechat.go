package pay

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/wechatpay-apiv3/wechatpay-go/core"
	"github.com/wechatpay-apiv3/wechatpay-go/core/auth/verifiers"
	"github.com/wechatpay-apiv3/wechatpay-go/core/downloader"
	"github.com/wechatpay-apiv3/wechatpay-go/core/notify"
	"github.com/wechatpay-apiv3/wechatpay-go/core/option"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments/app"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments/h5"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments/jsapi"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments/native"
	"github.com/wechatpay-apiv3/wechatpay-go/services/refunddomestic"
	"github.com/wechatpay-apiv3/wechatpay-go/utils"

	kitsettings "github.com/imbytecat/moonbase/integrations/core/settings"
)

// WeChat method ids are the official APIv3 trade types, matched by the driver's
// per-method dispatch below; the generated catalog (paymentcatalog) owns each
// method's credential shape.
const (
	wechatMethodNative = "native" // Native 扫码支付
	wechatMethodH5     = "h5"     // H5 支付
	wechatMethodJsapi  = "jsapi"  // JSAPI（公众号 / 小程序）支付
	wechatMethodApp    = "app"    // APP 支付
)

func wechatClient(ctx context.Context, config map[string]any) (*core.Client, error) {
	mchPriv, err := utils.LoadPrivateKey(cfgStr(config, "mchPrivateKey"))
	if err != nil {
		return nil, fmt.Errorf("load merchant private key: %w", err)
	}
	var opts []core.ClientOption
	mchID := cfgStr(config, "mchId")
	mchCertSerialNo := cfgStr(config, "mchCertSerialNo")
	apiV3Key := cfgStr(config, "apiV3Key")
	if cfgStr(config, "authMethod") == AuthPlatformCert {
		opts = append(opts, option.WithWechatPayAutoAuthCipher(mchID, mchCertSerialNo, mchPriv, apiV3Key))
	} else {
		wxPub, err := utils.LoadPublicKey(cfgStr(config, "publicKey"))
		if err != nil {
			return nil, fmt.Errorf("load wechatpay public key: %w", err)
		}
		opts = append(opts, option.WithWechatPayPublicKeyAuthCipher(mchID, mchCertSerialNo, mchPriv, cfgStr(config, "publicKeyId"), wxPub))
	}
	client, err := core.NewClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create wechatpay client: %w", err)
	}
	return client, nil
}

func wechatNotifyHandler(ctx context.Context, p kitsettings.GenericProfile) (*notify.Handler, error) {
	config := p.Config
	apiV3Key := cfgStr(config, "apiV3Key")
	mchID := cfgStr(config, "mchId")
	mchCertSerialNo := cfgStr(config, "mchCertSerialNo")
	if cfgStr(config, "authMethod") == AuthPlatformCert {
		mchPriv, err := utils.LoadPrivateKey(cfgStr(config, "mchPrivateKey"))
		if err != nil {
			return nil, fmt.Errorf("load merchant private key: %w", err)
		}
		mgr := downloader.MgrInstance()
		if err := mgr.RegisterDownloaderWithPrivateKey(ctx, mchPriv, mchCertSerialNo, mchID, apiV3Key); err != nil {
			return nil, fmt.Errorf("register certificate downloader: %w", err)
		}
		return notify.NewRSANotifyHandler(apiV3Key, verifiers.NewSHA256WithRSAVerifier(mgr.GetCertificateVisitor(mchID)))
	}
	wxPub, err := utils.LoadPublicKey(cfgStr(config, "publicKey"))
	if err != nil {
		return nil, fmt.Errorf("load wechatpay public key: %w", err)
	}
	return notify.NewRSANotifyHandler(apiV3Key, verifiers.NewSHA256WithRSAPubkeyVerifier(cfgStr(config, "publicKeyId"), *wxPub))
}

func wechatCreate(ctx context.Context, p kitsettings.GenericProfile, req CreateRequest, notifyURL string) (Credential, error) {
	client, err := wechatClient(ctx, p.Config)
	if err != nil {
		return "", err
	}
	appID := cfgStr(p.Config, "appId")
	mchID := cfgStr(p.Config, "mchId")
	switch req.Method {
	case wechatMethodNative:
		svc := native.NativeApiService{Client: client}
		rsp, _, err := svc.Prepay(ctx, native.PrepayRequest{
			Appid:       core.String(appID),
			Mchid:       core.String(mchID),
			Description: core.String(req.Subject),
			OutTradeNo:  core.String(req.OutTradeNo),
			NotifyUrl:   core.String(notifyURL),
			Amount:      &native.Amount{Total: core.Int64(req.Amount)},
		})
		if err != nil {
			return "", fmt.Errorf("wechat native prepay: %w", err)
		}
		if rsp.CodeUrl == nil {
			return "", fmt.Errorf("wechat native prepay: empty code_url")
		}
		return *rsp.CodeUrl, nil
	case wechatMethodH5:
		svc := h5.H5ApiService{Client: client}
		rsp, _, err := svc.Prepay(ctx, h5.PrepayRequest{
			Appid:       core.String(appID),
			Mchid:       core.String(mchID),
			Description: core.String(req.Subject),
			OutTradeNo:  core.String(req.OutTradeNo),
			NotifyUrl:   core.String(notifyURL),
			Amount:      &h5.Amount{Total: core.Int64(req.Amount)},
			SceneInfo: &h5.SceneInfo{
				PayerClientIp: core.String(req.ClientIP),
				H5Info:        &h5.H5Info{Type: core.String("Wap")},
			},
		})
		if err != nil {
			return "", fmt.Errorf("wechat h5 prepay: %w", err)
		}
		if rsp.H5Url == nil {
			return "", fmt.Errorf("wechat h5 prepay: empty h5_url")
		}
		return *rsp.H5Url, nil
	case wechatMethodJsapi:
		svc := jsapi.JsapiApiService{Client: client}
		rsp, _, err := svc.PrepayWithRequestPayment(ctx, jsapi.PrepayRequest{
			Appid:       core.String(appID),
			Mchid:       core.String(mchID),
			Description: core.String(req.Subject),
			OutTradeNo:  core.String(req.OutTradeNo),
			NotifyUrl:   core.String(notifyURL),
			Amount:      &jsapi.Amount{Total: core.Int64(req.Amount)},
			Payer:       &jsapi.Payer{Openid: core.String(req.PayerID)},
		})
		if err != nil {
			return "", fmt.Errorf("wechat jsapi prepay: %w", err)
		}
		raw, err := json.Marshal(rsp)
		if err != nil {
			return "", err
		}
		return string(raw), nil
	case wechatMethodApp:
		svc := app.AppApiService{Client: client}
		rsp, _, err := svc.PrepayWithRequestPayment(ctx, app.PrepayRequest{
			Appid:       core.String(appID),
			Mchid:       core.String(mchID),
			Description: core.String(req.Subject),
			OutTradeNo:  core.String(req.OutTradeNo),
			NotifyUrl:   core.String(notifyURL),
			Amount:      &app.Amount{Total: core.Int64(req.Amount)},
		})
		if err != nil {
			return "", fmt.Errorf("wechat app prepay: %w", err)
		}
		raw, err := json.Marshal(rsp)
		if err != nil {
			return "", err
		}
		return string(raw), nil
	default:
		return "", fmt.Errorf("wechat: unsupported method %q", req.Method)
	}
}

func wechatQuery(ctx context.Context, p kitsettings.GenericProfile, outTradeNo string) (QueryResult, error) {
	client, err := wechatClient(ctx, p.Config)
	if err != nil {
		return QueryResult{}, err
	}
	svc := native.NativeApiService{Client: client}
	tx, _, err := svc.QueryOrderByOutTradeNo(ctx, native.QueryOrderByOutTradeNoRequest{
		OutTradeNo: core.String(outTradeNo),
		Mchid:      core.String(cfgStr(p.Config, "mchId")),
	})
	if err != nil {
		return QueryResult{}, fmt.Errorf("wechat query: %w", err)
	}
	return wechatTransactionResult(tx), nil
}

func wechatRefund(ctx context.Context, p kitsettings.GenericProfile, req RefundRequest, notifyURL string) (RefundResult, error) {
	client, err := wechatClient(ctx, p.Config)
	if err != nil {
		return RefundResult{}, err
	}
	svc := refunddomestic.RefundsApiService{Client: client}
	rsp, _, err := svc.Create(ctx, refunddomestic.CreateRequest{
		OutTradeNo:  core.String(req.OutTradeNo),
		OutRefundNo: core.String(req.RefundNo),
		Reason:      core.String(req.Reason),
		NotifyUrl:   core.String(notifyURL),
		Amount: &refunddomestic.AmountReq{
			Refund:   core.Int64(req.Amount),
			Total:    core.Int64(req.Amount),
			Currency: core.String("CNY"),
		},
	})
	if err != nil {
		return RefundResult{}, fmt.Errorf("wechat refund: %w", err)
	}
	settled := rsp.Status != nil && *rsp.Status == refunddomestic.STATUS_SUCCESS
	return RefundResult{Settled: settled}, nil
}

func wechatQueryRefund(ctx context.Context, p kitsettings.GenericProfile, refundNo string) (bool, error) {
	client, err := wechatClient(ctx, p.Config)
	if err != nil {
		return false, err
	}
	svc := refunddomestic.RefundsApiService{Client: client}
	rsp, _, err := svc.QueryByOutRefundNo(ctx, refunddomestic.QueryByOutRefundNoRequest{
		OutRefundNo: core.String(refundNo),
	})
	if err != nil {
		return false, fmt.Errorf("wechat refund query: %w", err)
	}
	return rsp.Status != nil && *rsp.Status == refunddomestic.STATUS_SUCCESS, nil
}

func wechatParseNotify(ctx context.Context, p kitsettings.GenericProfile, r *http.Request) (NotifyResult, error) {
	handler, err := wechatNotifyHandler(ctx, p)
	if err != nil {
		return NotifyResult{}, err
	}
	// One resource shape for both notification families: TRANSACTION.*
	// carries a Transaction, REFUND.* carries a refund object — the fields
	// only partially overlap, so decode the superset.
	var res struct {
		OutTradeNo    string `json:"out_trade_no"`
		TransactionID string `json:"transaction_id"`
		TradeState    string `json:"trade_state"`
		SuccessTime   string `json:"success_time"`
		Payer         *struct {
			Openid string `json:"openid"`
		} `json:"payer"`
		RefundStatus string `json:"refund_status"`
	}
	req, err := handler.ParseNotifyRequest(ctx, r, &res)
	if err != nil {
		return NotifyResult{}, fmt.Errorf("wechat notify verify: %w", err)
	}
	result := QueryResult{State: StatePending, ProviderTradeNo: res.TransactionID}
	if res.Payer != nil {
		result.PayerID = res.Payer.Openid
	}
	if t, terr := time.Parse(time.RFC3339, res.SuccessTime); terr == nil {
		result.PaidAt = t
	}
	switch {
	case strings.HasPrefix(req.EventType, "REFUND."):
		if res.RefundStatus == "SUCCESS" {
			result.State = StateRefunded
		}
	default:
		result.State = wechatTradeState(res.TradeState)
	}
	return NotifyResult{
		OutTradeNo: res.OutTradeNo,
		State:      result.State,
		Query:      result,
		Ack: func(w http.ResponseWriter) {
			w.WriteHeader(http.StatusNoContent)
		},
	}, nil
}

func wechatTradeState(s string) State {
	switch s {
	case "SUCCESS":
		return StatePaid
	case "CLOSED", "REVOKED", "PAYERROR":
		return StateClosed
	case "REFUND":
		return StateRefunded
	default:
		return StatePending
	}
}

func wechatTransactionResult(tx *payments.Transaction) QueryResult {
	out := QueryResult{State: StatePending}
	if tx == nil || tx.TradeState == nil {
		return out
	}
	out.State = wechatTradeState(*tx.TradeState)
	if tx.TransactionId != nil {
		out.ProviderTradeNo = *tx.TransactionId
	}
	if tx.Payer != nil && tx.Payer.Openid != nil {
		out.PayerID = *tx.Payer.Openid
	}
	if tx.SuccessTime != nil {
		if t, err := time.Parse(time.RFC3339, *tx.SuccessTime); err == nil {
			out.PaidAt = t
		}
	}
	return out
}
