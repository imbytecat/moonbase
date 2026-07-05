package rpc

import (
	"net/http"
)

// PaymentNotify serves POST /payment/notify/{provider}/{profile}: the async
// settlement path. The driver verifies the provider's signature (that is the
// authentication — no session), then the status-guarded writes make replayed
// notifications idempotent. Providers retry on non-2xx, so transient
// failures answer 5xx and signature failures 4xx.
func (s *PaymentService) PaymentNotify(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	profileID := r.PathValue("profile")
	profile, err := s.gateway.ProfileByID(ctx, profileID)
	if err != nil || profile.Provider != r.PathValue("provider") {
		http.Error(w, "unknown payment profile", http.StatusNotFound)
		return
	}
	result, err := s.gateway.ParseNotify(ctx, profile, r)
	if err != nil {
		s.logger.WarnContext(ctx, "payment notification rejected", "profile", profileID, "error", err)
		http.Error(w, "invalid notification", http.StatusBadRequest)
		return
	}
	row, err := s.repo.GetPaymentOrderByOutTradeNo(ctx, result.OutTradeNo)
	if err != nil {
		s.logger.WarnContext(ctx, "payment notification for unknown order",
			"out_trade_no", result.OutTradeNo, "error", err)
		http.Error(w, "unknown order", http.StatusNotFound)
		return
	}
	if _, err := s.applyQueryResult(ctx, row, result.Query); err != nil {
		s.logger.ErrorContext(ctx, "payment notification apply failed",
			"out_trade_no", result.OutTradeNo, "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	result.Ack(w)
}
