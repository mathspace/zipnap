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
	"github.com/mathspace/zipnap/config"
	"github.com/mathspace/zipnap/host"
)

type EC2Host struct {
	cfg    config.Instance
	logger *log.Logger
	client *ec2.Client
}

func New(ctx context.Context, cfg config.Instance) (*EC2Host, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	return &EC2Host{
		cfg:    cfg,
		logger: log.New(log.Writer(), fmt.Sprintf("ec2-host(%s): ", cfg.EC2.InstanceID), 0),
		client: ec2.NewFromConfig(awsCfg),
	}, nil
}

func (h *EC2Host) Start(ctx context.Context) error {
	_, err := h.client.StartInstances(ctx, &ec2.StartInstancesInput{
		InstanceIds: []string{h.cfg.EC2.InstanceID},
	})
	return err
}

func (h *EC2Host) Stop(ctx context.Context) error {
	_, err := h.client.StopInstances(ctx, &ec2.StopInstancesInput{
		InstanceIds: []string{h.cfg.EC2.InstanceID},
		Hibernate:   aws.Bool(true),
	})
	return err
}

func (h *EC2Host) Status(ctx context.Context) (host.State, error) {
	st := host.State{}
	out, err := h.client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		InstanceIds: []string{h.cfg.EC2.InstanceID},
	})
	if err != nil {
		return st, err
	}
	if len(out.Reservations) != 1 || len(out.Reservations[0].Instances) != 1 {
		return st, fmt.Errorf("expected exactly one instance, got %d reservations and %d instances", len(out.Reservations), len(out.Reservations[0].Instances))
	}
	inst := out.Reservations[0].Instances[0]

	if inst.State == nil {
		st.Status = host.StatusUnknown
	} else {
		switch inst.State.Name {
		case ec2types.InstanceStateNamePending:
			st.Status = host.StatusStarting
		case ec2types.InstanceStateNameRunning:
			st.Status = host.StatusStarted
		case ec2types.InstanceStateNameStopping:
			st.Status = host.StatusStopping
		case ec2types.InstanceStateNameStopped:
			st.Status = host.StatusStopped
		default:
			st.Status = host.StatusUnknown
		}
	}

	if inst.PrivateIpAddress != nil {
		st.HostName = *inst.PrivateIpAddress
	}

	return st, nil
}
