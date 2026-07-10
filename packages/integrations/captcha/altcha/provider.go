package altcha

import (
	"context"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	lib "github.com/altcha-org/altcha-lib-go"
	captchaint "github.com/imbytecat/moonbase/integrations/captcha"
	"github.com/imbytecat/moonbase/integrations/core/config"
	"github.com/imbytecat/moonbase/integrations/core/integration"
)

const defaultMaxNumber = 1_000_000
const challengeTTL = 10 * time.Minute

type providerConfig struct {
	Difficulty int64 `json:"difficulty,omitempty" jsonschema:"title=难度,minimum=0,maximum=10000000"`
}
type replayCache struct {
	mu   sync.Mutex
	seen map[string]time.Time
}

func New(load func(context.Context) ([]byte, error)) captchaint.Registration {
	replay := &replayCache{seen: map[string]time.Time{}}
	key := func(ctx context.Context) (string, error) {
		raw, err := load(ctx)
		if err != nil {
			return "", err
		}
		return hex.EncodeToString(raw), nil
	}
	return captchaint.Register("altcha", integration.Presentation{Name: "内置工作量验证", Description: "由本站签发并校验无外部依赖的计算挑战", Color: "#52c41a", IconRef: "antd:ThunderboltOutlined"}, config.MustContract[providerConfig](config.Policy{}), captchaint.Operations[providerConfig]{SiteKey: func(providerConfig) string { return "" }, Challenge: func(ctx context.Context, c providerConfig) (any, error) {
		k, err := key(ctx)
		if err != nil {
			return nil, err
		}
		max := c.Difficulty
		if max <= 0 {
			max = defaultMaxNumber
		}
		expires := time.Now().Add(challengeTTL)
		ch, err := lib.CreateChallenge(lib.ChallengeOptions{Algorithm: lib.SHA256, HMACKey: k, MaxNumber: max, Expires: &expires})
		if err != nil {
			return nil, fmt.Errorf("create altcha challenge: %w", err)
		}
		return ch, nil
	}, Verify: func(ctx context.Context, _ providerConfig, token, _ string) error {
		k, err := key(ctx)
		if err != nil {
			return err
		}
		ok, err := lib.VerifySolutionSafe(token, k, true)
		if err != nil {
			return fmt.Errorf("altcha verify: %w", err)
		}
		if !ok || !replay.consume(token, time.Now().Add(challengeTTL)) {
			return fmt.Errorf("captcha verification failed")
		}
		return nil
	}})
}
func (r *replayCache) consume(id string, expires time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	for k, exp := range r.seen {
		if now.After(exp) {
			delete(r.seen, k)
		}
	}
	if _, ok := r.seen[id]; ok {
		return false
	}
	r.seen[id] = expires
	return true
}
