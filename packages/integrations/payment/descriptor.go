package pay

import (
	"fmt"
	"slices"
	"strings"

	"github.com/imbytecat/moonbase/integrations/core/form"
	"github.com/imbytecat/moonbase/integrations/core/integration"
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

func PlanConfigured(
	descriptor ProviderDescriptor,
	provider string,
	configured []string,
	request PlanRequest,
) (PlanResult, error) {
	if !slices.ContainsFunc(descriptor.Methods, func(method PaymentMethodDescriptor) bool {
		return method.Key == request.PaymentMethod
	}) {
		return PlanResult{}, fmt.Errorf("%w: %q", ErrUnknownMethod, request.PaymentMethod)
	}

	offered := configuredProducts(configured, descriptor.Products)
	for _, productID := range productPriority(provider, request.Client.UserAgent) {
		if product := productByID(
			offered,
			productID,
		); product != nil &&
			product.Method == request.PaymentMethod {
			return PlanResult{ProductID: product.ID, Input: product.Input}, nil
		}
	}
	return PlanResult{}, fmt.Errorf("%w: %q", ErrMethodNotOffered, request.PaymentMethod)
}

func configuredProducts(signed []string, catalog []ProductDescriptor) []ProductDescriptor {
	if len(signed) == 0 {
		return slices.Clone(catalog)
	}
	return slices.DeleteFunc(slices.Clone(catalog), func(product ProductDescriptor) bool {
		return !slices.Contains(signed, product.ID)
	})
}

func productPriority(provider, userAgent string) []string {
	mobile := strings.Contains(userAgent, "Mobile") || strings.Contains(userAgent, "Android") ||
		strings.Contains(userAgent, "iPhone")
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
