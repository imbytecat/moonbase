package pay

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/smartwalle/alipay/v3"

	"github.com/imbytecat/moonbase/server/integrationkit/systemcodec"
	"github.com/imbytecat/moonbase/server/internal/settings"
)

// Alipay method ids are the official API-method names, matched by the driver's
// per-method dispatch below; the generated catalog (paymentcatalog) owns each
// method's credential shape and inputs, and alipayProduct* the sales
// product_code it sends to the gateway.
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

func alipayUsable(p systemcodec.PaymentProfile) bool {
	a := p.Alipay
	if a.AppId == "" || a.AppPrivateKey == "" {
		return false
	}
	switch a.AuthMethod {
	case settings.PaymentAuthCert:
		return a.AppCert != "" && a.AlipayRootCert != "" && a.AlipayPublicCert != ""
	default:
		return a.AlipayPublicKey != ""
	}
}

func alipayClient(p systemcodec.PaymentProfile) (*alipay.Client, error) {
	a := p.Alipay
	client, err := alipay.New(a.AppId, a.AppPrivateKey, true)
	if err != nil {
		return nil, fmt.Errorf("create alipay client: %w", err)
	}
	if a.AuthMethod == settings.PaymentAuthCert {
		if err := client.LoadAppCertPublicKey(a.AppCert); err != nil {
			return nil, fmt.Errorf("load app cert: %w", err)
		}
		if err := client.LoadAliPayRootCert(a.AlipayRootCert); err != nil {
			return nil, fmt.Errorf("load alipay root cert: %w", err)
		}
		if err := client.LoadAlipayCertPublicKey(a.AlipayPublicCert); err != nil {
			return nil, fmt.Errorf("load alipay public cert: %w", err)
		}
		return client, nil
	}
	if err := client.LoadAliPayPublicKey(a.AlipayPublicKey); err != nil {
		return nil, fmt.Errorf("load alipay public key: %w", err)
	}
	return client, nil
}

func alipayCreate(ctx context.Context, p systemcodec.PaymentProfile, req CreateRequest, notifyURL string) (Credential, error) {
	client, err := alipayClient(p)
	if err != nil {
		return "", err
	}
	switch req.Method {
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
		if p.Alipay.OpAppId == "" {
			return "", fmt.Errorf("%w: op_app_id for alipay jsapi", ErrMissingInput)
		}
		var param alipay.TradeCreate
		param.OutTradeNo = req.OutTradeNo
		param.Subject = req.Subject
		param.TotalAmount = yuan(req.Amount)
		param.ProductCode = alipayProductJSAPI
		param.OpAppId = p.Alipay.OpAppId
		param.NotifyURL = notifyURL
		// buyer_id is the legacy 2088-prefixed numeric user id; anything else
		// is the newer openid shape.
		if strings.HasPrefix(req.PayerID, "2088") {
			param.BuyerId = req.PayerID
		} else {
			param.BuyerOpenId = req.PayerID
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
		return "", fmt.Errorf("alipay: unsupported method %q", req.Method)
	}
}

func alipayQuery(ctx context.Context, p systemcodec.PaymentProfile, outTradeNo string) (QueryResult, error) {
	client, err := alipayClient(p)
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
			return QueryResult{State: StatePending}, nil
		}
		return QueryResult{}, fmt.Errorf("alipay query: %s (%s)", rsp.SubMsg, rsp.SubCode)
	}
	return QueryResult{
		State:           alipayState(rsp.TradeStatus),
		ProviderTradeNo: rsp.TradeNo,
		PayerID:         alipayPayer(rsp.BuyerOpenId, rsp.BuyerUserId),
		PaidAt:          alipayTime(rsp.SendPayDate),
	}, nil
}

func alipayRefund(ctx context.Context, p systemcodec.PaymentProfile, req RefundRequest, _ string) (RefundResult, error) {
	client, err := alipayClient(p)
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

func alipayQueryRefund(_ context.Context, _ systemcodec.PaymentProfile, _ string) (bool, error) {
	// Alipay refunds settle synchronously in TradeRefund; nothing to poll.
	return true, nil
}

func alipayParseNotify(ctx context.Context, p systemcodec.PaymentProfile, r *http.Request) (NotifyResult, error) {
	client, err := alipayClient(p)
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
