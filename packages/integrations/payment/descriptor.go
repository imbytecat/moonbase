package pay

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/imbytecat/moonbase/integrations/core/form"
	"github.com/imbytecat/moonbase/integrations/core/integration"
	kitsettings "github.com/imbytecat/moonbase/integrations/core/settings"
)

type PaymentMethodDescriptor struct {
	Key          string
	Presentation integration.Presentation
}

type ProductDescriptor struct {
	ID           string
	Method       string
	Presentation integration.Presentation
	Input        form.Schema
}

type ProviderDescriptor struct {
	Methods      []PaymentMethodDescriptor
	Products     []ProductDescriptor
	Capabilities []string
}

type ClientContext struct {
	UserAgent string
	IP        string
}

type PlanRequest struct {
	PaymentMethod string
	Client        ClientContext
}

type PlanResult struct {
	ProductID string
	Input     form.Schema
}

var alipayDescriptor = ProviderDescriptor{
	Methods: []PaymentMethodDescriptor{{
		Key: "alipay",
		Presentation: integration.Presentation{
			Name: "支付宝", Description: "使用支付宝完成付款", Color: "#1677ff", IconRef: "antd:AlipayCircleOutlined",
		},
	}},
	Products: []ProductDescriptor{
		{ID: "precreate", Method: "alipay", Presentation: productPresentation("当面付二维码", "由付款人扫码完成支付")},
		{ID: "page_pay", Method: "alipay", Presentation: productPresentation("电脑网站支付", "跳转到电脑端支付页面")},
		{ID: "wap_pay", Method: "alipay", Presentation: productPresentation("手机网站支付", "跳转到移动端支付页面")},
		{ID: "create", Method: "alipay", Presentation: productPresentation("小程序支付", "在小程序内唤起支付"), Input: payerInput("买家标识")},
		{ID: "app_pay", Method: "alipay", Presentation: productPresentation("App 支付", "在移动应用内唤起支付")},
	},
}

var wechatDescriptor = ProviderDescriptor{
	Methods: []PaymentMethodDescriptor{{
		Key: "wechat",
		Presentation: integration.Presentation{
			Name: "微信支付", Description: "使用微信支付完成付款", Color: "#07c160", IconRef: "antd:WechatOutlined",
		},
	}},
	Products: []ProductDescriptor{
		{ID: "native", Method: "wechat", Presentation: productPresentation("Native 支付", "由付款人扫码完成支付")},
		{ID: "h5", Method: "wechat", Presentation: productPresentation("H5 支付", "跳转到移动端支付页面")},
		{ID: "jsapi", Method: "wechat", Presentation: productPresentation("JSAPI 支付", "在微信内唤起支付"), Input: payerInput("用户标识")},
		{ID: "app", Method: "wechat", Presentation: productPresentation("App 支付", "在移动应用内唤起支付")},
	},
}

func productPresentation(name, description string) integration.Presentation {
	return integration.Presentation{Name: name, Description: description}
}

func payerInput(label string) form.Schema {
	return form.Schema{Fields: []form.Field{{Key: "payer_id", Label: label, Type: form.String, Required: true, MaxLen: 128}}}
}

func Describe(provider string) (ProviderDescriptor, bool) {
	entry, ok := Registry.EntryFor(provider)
	if !ok {
		return ProviderDescriptor{}, false
	}
	descriptor := entry.Ops.Describe()
	descriptor.Capabilities = driverCapabilities(entry.Ops)
	return descriptor, true
}

func Plan(ctx context.Context, profile kitsettings.GenericProfile, request PlanRequest) (PlanResult, error) {
	driver, ok := Registry.OpsFor(profile.Provider, profile.Config)
	if !ok {
		return PlanResult{}, ErrNotConfigured
	}
	return driver.Plan(ctx, profile, request)
}

func plan(descriptor ProviderDescriptor, profile kitsettings.GenericProfile, request PlanRequest) (PlanResult, error) {
	if !slices.ContainsFunc(descriptor.Methods, func(method PaymentMethodDescriptor) bool {
		return method.Key == request.PaymentMethod
	}) {
		return PlanResult{}, fmt.Errorf("%w: %q", ErrUnknownMethod, request.PaymentMethod)
	}

	offered := offeredProducts(profile, descriptor.Products)
	for _, productID := range productPriority(profile.Provider, request.Client.UserAgent) {
		if product := productByID(offered, productID); product != nil && product.Method == request.PaymentMethod {
			return PlanResult{ProductID: product.ID, Input: product.Input}, nil
		}
	}
	return PlanResult{}, fmt.Errorf("%w: %q", ErrMethodNotOffered, request.PaymentMethod)
}

func offeredProducts(profile kitsettings.GenericProfile, catalog []ProductDescriptor) []ProductDescriptor {
	signed := profileConfiguredProducts(profile)
	if len(signed) == 0 {
		return slices.Clone(catalog)
	}
	return slices.DeleteFunc(slices.Clone(catalog), func(product ProductDescriptor) bool {
		return !slices.Contains(signed, product.ID)
	})
}

func productPriority(provider, userAgent string) []string {
	mobile := strings.Contains(userAgent, "Mobile") || strings.Contains(userAgent, "Android") || strings.Contains(userAgent, "iPhone")
	switch provider {
	case "alipay":
		if mobile {
			return []string{"wap_pay", "page_pay", "precreate", "create", "app_pay"}
		}
		return []string{"page_pay", "precreate", "wap_pay", "create", "app_pay"}
	case "wechat":
		if strings.Contains(userAgent, "MicroMessenger") {
			return []string{"jsapi", "native", "h5", "app"}
		}
		if mobile {
			return []string{"h5", "native", "jsapi", "app"}
		}
		return []string{"native", "h5", "jsapi", "app"}
	default:
		return nil
	}
}

func productByID(products []ProductDescriptor, id string) *ProductDescriptor {
	for i := range products {
		if products[i].ID == id {
			return &products[i]
		}
	}
	return nil
}
