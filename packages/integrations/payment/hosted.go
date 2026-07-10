package pay

import (
	"encoding/json"
	"fmt"
)

// RenderHostedFlow returns provider-owned same-origin HTML for products whose
// client lifecycle cannot be represented by QR, redirect, or form actions.
func renderHostedFlow(provider, product, payload string) ([]byte, error) {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	var script string
	switch provider + ":" + product {
	case "wechat:jsapi":
		script = fmt.Sprintf(`
const params = JSON.parse(%s)
function invoke() {
  WeixinJSBridge.invoke('getBrandWCPayRequest', params, () => window.close())
}
if (typeof WeixinJSBridge === 'undefined') {
  document.addEventListener('WeixinJSBridgeReady', invoke, false)
} else { invoke() }
`, payloadJSON)
	case "alipay:create":
		script = fmt.Sprintf(`
const params = JSON.parse(%s)
if (window.my && window.my.tradePay) {
  window.my.tradePay({ tradeNO: params.tradeNo }, () => window.close())
}
`, payloadJSON)
	default:
		script = `document.getElementById('status').textContent = '请在支持该支付方式的应用中继续完成付款'`
	}
	html := `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>继续支付</title></head><body><main><h1>继续支付</h1><p id="status">正在唤起支付…</p></main><script>` + script + `</script></body></html>`
	return []byte(html), nil
}
