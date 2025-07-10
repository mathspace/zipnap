// Package httpproxy provides a HTTP proxy implementation for a service.
package httpproxy

import (
	"context"

	"github.com/mathspace/zipnap/config"
)

type HTTPProxy struct {
	cfg config.HTTP
}

func New(c config.HTTP) *HTTPProxy {
	return &HTTPProxy{
		cfg: c,
	}
}

func (p *HTTPProxy) Start(waitHealthy func(ctx context.Context) error) error {
}

func (p *HTTPProxy) IsServiceHealthy(ctx context.Context, host string) (bool, error) {
}

func (p *HTTPProxy) Kill(ctx context.Context) error {
}
