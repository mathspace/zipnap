// Package ec2host provides an implementation of the host.Host interface for AWS
// EC2 instances.
package ec2host

import (
	"context"

	"github.com/mathspace/zipnap/config"
	"github.com/mathspace/zipnap/host"
)

type EC2Host struct {
	cfg config.Instance
}

func New(cfg config.Instance) *EC2Host {
	return &EC2Host{
		cfg: cfg,
	}
}

func (h *EC2Host) Start(ctx context.Context) error {
	return nil
}

func (h *EC2Host) Stop(ctx context.Context) error {
	return nil
}

func (h *EC2Host) Status(ctx context.Context) (host.Status, error) {
	return host.StatusUnknown, nil
}
