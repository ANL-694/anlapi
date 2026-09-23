package service

import (
	"context"
	"time"
)

// PrivateGatewayNonceRepository is the narrow durable port used by the
// private service-authentication middleware. Implementations must claim a
// nonce atomically across processes and reclaim only expired claims.
type PrivateGatewayNonceRepository interface {
	ClaimPrivateGatewayNonce(ctx context.Context, scope, nonceHash string, expiresAt time.Time) (bool, error)
}
