// Package ec2host provides an implementation of the host.Host interface for AWS
// EC2 instances.
package ec2host

import (
	"context"
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/mathspace/zipnap/host"
)

// EC2Host implements the host.Host interface for managing an AWS EC2 instance.
type EC2Host struct {
	cfg    *Config
	logger *log.Logger
	client *ec2.Client
}

// New creates a new EC2Host instance using the provided context, configuration,
// and logger.
func New(ctx context.Context, cfg *Config, logger *log.Logger) (*EC2Host, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	return &EC2Host{
		cfg:    cfg,
		logger: logger,
		client: ec2.NewFromConfig(awsCfg),
	}, nil
}

// Start starts the EC2 instance specified in the configuration.
func (h *EC2Host) Start(ctx context.Context) error {
	_, err := h.client.StartInstances(ctx, &ec2.StartInstancesInput{
		InstanceIds: []string{h.cfg.InstanceID},
	})
	return err
}

// Stop stops the EC2 instance specified in the configuration, with hibernation
// enabled (if available).
func (h *EC2Host) Stop(ctx context.Context) error {
	_, err := h.client.StopInstances(ctx, &ec2.StopInstancesInput{
		InstanceIds: []string{h.cfg.InstanceID},
		Hibernate:   aws.Bool(true),
	})
	return err
}

// State retrieves the current state of the EC2 instance specified in the
// configuration.
func (h *EC2Host) State(ctx context.Context) (host.State, error) {
	st := host.State{Status: host.StatusUnknown}

	resp, err := h.client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		InstanceIds: []string{h.cfg.InstanceID},
	})
	if err != nil {
		return st, err
	}
	if len(resp.Reservations) == 0 || len(resp.Reservations[0].Instances) == 0 {
		return st, fmt.Errorf("no instances found for ID %s", h.cfg.InstanceID)
	}
	inst := resp.Reservations[0].Instances[0]

	if inst.State == nil {
		return st, nil
	}

	switch inst.State.Name {
	case ec2types.InstanceStateNameRunning:
		st.Status = host.StatusStarted
		if inst.PrivateIpAddress != nil {
			st.Addr = *inst.PrivateIpAddress
		}
	case ec2types.InstanceStateNamePending:
		st.Status = host.StatusStarting
	case ec2types.InstanceStateNameStopping:
		st.Status = host.StatusStopping
	case ec2types.InstanceStateNameStopped:
		st.Status = host.StatusStopped
	}

	return st, nil
}
