package rpc

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/imbytecat/moonbase/server/internal/pay"
)

func (s *PaymentService) HostedFlow(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("session")
	session, err := s.core.checkout.Resolve(r.Context(), token)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	order, err := s.core.repo.GetPaymentOrderByCheckoutSession(
		r.Context(),
		pgtype.Text{String: session.ID, Valid: true},
	)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	var action pay.Action
	if json.Unmarshal(order.Action, &action) != nil || action.HostedFlow == nil {
		http.NotFound(w, r)
		return
	}
	body, err := s.core.gateway.RenderHostedFlow(
		order.Provider,
		order.ProductID,
		action.HostedFlow.Payload,
	)
	if err != nil {
		http.Error(w, "无法加载支付页面", http.StatusInternalServerError)
		return
	}
	var nonceBytes [16]byte
	_, _ = rand.Read(nonceBytes[:])
	nonce := base64.RawStdEncoding.EncodeToString(nonceBytes[:])
	body = []byte(strings.Replace(string(body), "<script>", `<script nonce="`+nonce+`">`, 1))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().
		Set("Content-Security-Policy", "default-src 'none'; script-src 'nonce-"+nonce+"'; style-src 'unsafe-inline'; base-uri 'none'; frame-ancestors 'self'")
	_, _ = w.Write(body)
}
