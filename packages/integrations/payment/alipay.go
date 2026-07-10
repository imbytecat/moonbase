package pay

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/smartwalle/alipay/v3"

	kitsettings "github.com/imbytecat/moonbase/integrations/core/settings"
)

// Alipay product ids are the official API-method names. The driver descriptor
// owns their presentation and input schema; the dispatch below owns the API
// call and the product_code sent to the provider.
const (
	alipayMethodPreCreate = "precreate" // 当面付/订单码 扫码 · alipay.trade.precreate
	alipayMethodPagePay   = "page_pay"  // 电脑网站支付 · alipay.trade.page.pay
	alipayMethodWapPay    = "wap_pay"   // 手机网站支付 · alipay.trade.wap.pay
	alipayMethodCreate    = "create"    // 小程序 JSAPI 支付 · alipay.trade.create
	alipayMethodAppPay    = "app_pay"   // APP 支付 · alipay.trade.app.pay
)

const (
	alipayProductFaceToFace = "FACE_TO_FACE_PAYMENT"
	alipayProductPage       = "FAST_INSTANT_TRADE_PAY"
	alipayProductWap        = "QUICK_WAP_WAY"
	alipayProductJSAPI      = "JSAPI_PAY"
	alipayProductApp        = "QUICK_MSECURITY_PAY"
)

func alipayClient(config map[string]any) (*alipay.Client, error) {
	client, err := alipay.New(cfgStr(config, "appId"), cfgStr(config, "appPrivateKey"), true)
	if err != nil {
		return nil, fmt.Errorf("create alipay client: %w", err)
	}
	if cfgStr(config, "authMethod") == AuthCert {
		if err := client.LoadAppCertPublicKey(cfgStr(config, "appCert")); err != nil {
			return nil, fmt.Errorf("load app cert: %w", err)
		}
		if err := client.LoadAliPayRootCert(cfgStr(config, "alipayRootCert")); err != nil {
			return nil, fmt.Errorf("load alipay root cert: %w", err)
		}
		if err := client.LoadAlipayCertPublicKey(cfgStr(config, "alipayPublicCert")); err != nil {
			return nil, fmt.Errorf("load alipay public cert: %w", err)
		}
		return client, nil
	}
	if err := client.LoadAliPayPublicKey(cfgStr(config, "alipayPublicKey")); err != nil {
		return nil, fmt.Errorf("load alipay public key: %w", err)
	}
	return client, nil
}

func alipayCreate(ctx context.Context, p kitsettings.GenericProfile, req CreateRequest, notifyURL string) (string, error) {
	client, err := alipayClient(p.Config)
	if err != nil {
		return "", err
	}
	switch req.ProductID {
	case alipayMethodPreCreate:
		var param alipay.TradePreCreate
		param.OutTradeNo = req.OutTradeNo
		param.Subject = req.Subject
		param.TotalAmount = yuan(req.Amount)
		param.ProductCode = alipayProductFaceToFace
		param.NotifyURL = notifyURL
		rsp, err := client.TradePreCreate(ctx, param)
		if err != nil {
			return "", fmt.Errorf("alipay precreate: %w", err)
		}
		if rsp.IsFailure() {
			return "", fmt.Errorf("alipay precreate: %s (%s)", rsp.SubMsg, rsp.SubCode)
		}
		return rsp.QRCode, nil
	case alipayMethodPagePay:
		var param alipay.TradePagePay
		param.OutTradeNo = req.OutTradeNo
		param.Subject = req.Subject
		param.TotalAmount = yuan(req.Amount)
		param.ProductCode = alipayProductPage
		param.NotifyURL = notifyURL
		param.ReturnURL = req.ReturnURL
		u, err := client.TradePagePay(param)
		if err != nil {
			return "", fmt.Errorf("alipay page pay: %w", err)
		}
		return u.String(), nil
	case alipayMethodWapPay:
		var param alipay.TradeWapPay
		param.OutTradeNo = req.OutTradeNo
		param.Subject = req.Subject
		param.TotalAmount = yuan(req.Amount)
		param.ProductCode = alipayProductWap
		param.NotifyURL = notifyURL
		param.ReturnURL = req.ReturnURL
		u, err := client.TradeWapPay(param)
		if err != nil {
			return "", fmt.Errorf("alipay wap pay: %w", err)
		}
		return u.String(), nil
	case alipayMethodCreate:
		opAppID := cfgStr(p.Config, "opAppId")
		if opAppID == "" {
			return "", fmt.Errorf("%w: op_app_id for alipay jsapi", ErrMissingInput)
		}
		var param alipay.TradeCreate
		param.OutTradeNo = req.OutTradeNo
		param.Subject = req.Subject
		param.TotalAmount = yuan(req.Amount)
		param.ProductCode = alipayProductJSAPI
		param.OpAppId = opAppID
		param.NotifyURL = notifyURL
		// buyer_id is the legacy 2088-prefixed numeric user id; anything else
		// is the newer openid shape.
		payerID := inputString(req.Inputs, "payer_id")
		if strings.HasPrefix(payerID, "2088") {
			param.BuyerId = payerID
		} else {
			param.BuyerOpenId = payerID
		}
		rsp, err := client.TradeCreate(ctx, param)
		if err != nil {
			return "", fmt.Errorf("alipay trade create: %w", err)
		}
		if rsp.IsFailure() {
			return "", fmt.Errorf("alipay trade create: %s (%s)", rsp.SubMsg, rsp.SubCode)
		}
		raw, err := json.Marshal(map[string]string{"tradeNo": rsp.TradeNo})
		if err != nil {
			return "", err
		}
		return string(raw), nil
	case alipayMethodAppPay:
		var param alipay.TradeAppPay
		param.OutTradeNo = req.OutTradeNo
		param.Subject = req.Subject
		param.TotalAmount = yuan(req.Amount)
		param.ProductCode = alipayProductApp
		param.NotifyURL = notifyURL
		orderStr, err := client.TradeAppPay(param)
		if err != nil {
			return "", fmt.Errorf("alipay app pay: %w", err)
		}
		return orderStr, nil
	default:
		return "", fmt.Errorf("alipay: unsupported product %q", req.ProductID)
	}
}

func alipayQuery(ctx context.Context, p kitsettings.GenericProfile, outTradeNo string) (QueryResult, error) {
	client, err := alipayClient(p.Config)
	if err != nil {
		return QueryResult{}, err
	}
	rsp, err := client.TradeQuery(ctx, alipay.TradeQuery{OutTradeNo: outTradeNo})
	if err != nil {
		return QueryResult{}, fmt.Errorf("alipay query: %w", err)
	}
	// ACQ.TRADE_NOT_EXIST means the buyer never scanned — still pending.
	if rsp.IsFailure() {
		if rsp.SubCode == "ACQ.TRADE_NOT_EXIST" {
			return QueryResult{Exists: false, State: StatePending}, nil
		}
		return QueryResult{}, fmt.Errorf("alipay query: %s (%s)", rsp.SubMsg, rsp.SubCode)
	}
	return QueryResult{
		Exists:          true,
		State:           alipayState(rsp.TradeStatus),
		ProviderTradeNo: rsp.TradeNo,
		PayerID:         alipayPayer(rsp.BuyerOpenId, rsp.BuyerUserId),
		PaidAt:          alipayTime(rsp.SendPayDate),
	}, nil
}

func alipayRefund(ctx context.Context, p kitsettings.GenericProfile, req RefundRequest, _ string) (RefundResult, error) {
	client, err := alipayClient(p.Config)
	if err != nil {
		return RefundResult{}, err
	}
	rsp, err := client.TradeRefund(ctx, alipay.TradeRefund{
		OutTradeNo:   req.OutTradeNo,
		OutRequestNo: req.RefundNo,
		RefundAmount: yuan(req.Amount),
		RefundReason: req.Reason,
	})
	if err != nil {
		return RefundResult{}, fmt.Errorf("alipay refund: %w", err)
	}
	if rsp.IsFailure() {
		return RefundResult{}, fmt.Errorf("alipay refund: %s (%s)", rsp.SubMsg, rsp.SubCode)
	}
	return RefundResult{Settled: true}, nil
}

func alipayQueryRefund(_ context.Context, _ kitsettings.GenericProfile, _ string) (bool, error) {
	// Alipay refunds settle synchronously in TradeRefund; nothing to poll.
	return true, nil
}

func alipayParseNotify(ctx context.Context, p kitsettings.GenericProfile, r *http.Request) (NotifyResult, error) {
	client, err := alipayClient(p.Config)
	if err != nil {
		return NotifyResult{}, err
	}
	if err := r.ParseForm(); err != nil {
		return NotifyResult{}, fmt.Errorf("alipay notify: %w", err)
	}
	n, err := client.DecodeNotification(ctx, r.Form)
	if err != nil {
		return NotifyResult{}, fmt.Errorf("alipay notify verify: %w", err)
	}
	state := alipayState(n.TradeStatus)
	return NotifyResult{
		OutTradeNo: n.OutTradeNo,
		State:      state,
		Query: QueryResult{
			Exists:          true,
			State:           state,
			ProviderTradeNo: n.TradeNo,
			PayerID:         alipayPayer(n.BuyerOpenId, n.BuyerId),
			PaidAt:          alipayTime(n.GmtPayment),
		},
		Ack: client.ACKNotification,
	}, nil
}

func alipayState(s alipay.TradeStatus) State {
	switch s {
	case alipay.TradeStatusSuccess, alipay.TradeStatusFinished:
		return StatePaid
	case alipay.TradeStatusClosed:
		return StateClosed
	default:
		return StatePending
	}
}

func alipayPayer(openID, userID string) string {
	if openID != "" {
		return openID
	}
	return userID
}

// alipayTime parses the provider's "2006-01-02 15:04:05" timestamps, which
// are Beijing time regardless of server locale.
func alipayTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	loc := time.FixedZone("CST", 8*3600)
	t, err := time.ParseInLocation(time.DateTime, s, loc)
	if err != nil {
		return time.Time{}
	}
	return t
}
