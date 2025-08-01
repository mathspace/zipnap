// Package rdshost provides an implementation of the host.Host interface for AWS
// RDS instances.
package rdshost

import (
	"context"
	"log"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/mathspace/zipnap/host"
)

// RDSHost implements the host.Host interface for managing an AWS RDS instance.
type RDSHost struct {
	cfg    *Config
	logger *log.Logger
	client *rds.Client
}

// New creates a new RDSHost instance using the provided context, configuration,
// and logger.
func New(ctx context.Context, cfg *Config, logger *log.Logger) (*RDSHost, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	return &RDSHost{
		cfg:    cfg,
		logger: logger,
		client: rds.NewFromConfig(awsCfg),
	}, nil
}

// Start starts the RDS instance specified in the configuration.
func (h *RDSHost) Start(ctx context.Context) error {
	_, err := h.client.StartDBInstance(ctx, &rds.StartDBInstanceInput{
		DBInstanceIdentifier: aws.String(h.cfg.InstanceID),
	})
	return err
}

// Stop stops the RDS instance specified in the configuration, with hibernation
// enabled (if available).
func (h *RDSHost) Stop(ctx context.Context) error {
	_, err := h.client.StopDBInstance(ctx, &rds.StopDBInstanceInput{
		DBInstanceIdentifier: aws.String(h.cfg.InstanceID),
	})
	return err
}

// State retrieves the current state of the RDS instance specified in the
// configuration.
func (h *RDSHost) State(ctx context.Context) (host.State, error) {
	st := host.State{Status: host.StatusUnknown}

	resp, err := h.client.DescribeDBInstances(ctx, &rds.DescribeDBInstancesInput{
		DBInstanceIdentifier: aws.String(h.cfg.InstanceID),
	})
	if err != nil {
		return st, err
	}
	inst := resp.DBInstances[0]

	if inst.DBInstanceStatus == nil {
		return st, nil
	}

	switch *inst.DBInstanceStatus {
	case "starting":
		st.Status = host.StatusStarting
	case "stopping":
		st.Status = host.StatusStopping
	case "stopped":
		st.Status = host.StatusStopped
	default:
		// There are multitude of statuses with no clear indication of whether
		// the DB is available or not. We only want to deal with statuses for
		// which we are likely responsible.
		st.Status = host.StatusStarted
		if inst.Endpoint != nil && inst.Endpoint.Address != nil {
			st.Addr = *inst.Endpoint.Address
		}
	}

	return st, nil
}
