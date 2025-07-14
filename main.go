package main

import (
	"context"
	_ "embed"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/robfig/cron/v3"

	"github.com/mathspace/zipnap/config"
	"github.com/mathspace/zipnap/host"
	"github.com/mathspace/zipnap/host/ec2host"
	"github.com/mathspace/zipnap/proxy"
	"github.com/mathspace/zipnap/proxy/httpproxy"
)

type instanceProxy struct {
	p                proxy.Proxy
	lastPing         atomic.Bool
	activeConns      atomic.Int32
	lastActivityTime atomic.Value // time.Time
	logger           *log.Logger
}

type instance struct {
	cfg           config.Instance
	proxies       map[string]*instanceProxy
	schedules     map[string]*cron.Cron
	host          host.Host    // Host interface for managing the EC2 instance
	lastHostState atomic.Value // host.State
	wakupCh       chan struct{}
	hostReadyCond *sync.Cond // Condition variable to signal when the host is ready
	logger        *log.Logger
}

func newInstance(ctx context.Context, id string, cfg config.Instance) (*instance, error) {
	proxies := make(map[string]*instanceProxy, len(cfg.Services))
	for svcID, svc := range cfg.Services {
		logger := log.New(os.Stdout, fmt.Sprintf("instance[%s] proxy[%s]: ", svcID), 0)
		var p proxy.Proxy
		switch svc.Type {
		case config.ServiceTypeHTTP:
			p = httpproxy.New(svc, logger)
		default:
			return nil, fmt.Errorf("unsupported service type %s for service %s", svc.Type, svcID)
		}
		proxies[svcID] = &instanceProxy{
			p:      p,
			logger: logger,
		}
	}
	if cfg.Type != config.InstanceTypeEC2 {
		return nil, fmt.Errorf("unsupported instance type %s", cfg.Type)
	}
	logger := log.New(os.Stdout, fmt.Sprintf("instance[%s]: ", id), 0)
	h, err := ec2host.New(ctx, cfg, logger)
	if err != nil {
		return nil, err
	}

	inst := &instance{
		cfg:           cfg,
		proxies:       proxies,
		schedules:     nil,
		wakupCh:       make(chan struct{}, 1),
		host:          h,
		hostReadyCond: sync.NewCond(&sync.Mutex{}),
	}
	inst.lastHostState.Store(host.State{Status: host.StatusUnknown})
	return inst, nil
}

func (i *instance) runProxies(ctx context.Context) error {
	innerCtx, cancel := context.WithCancelCause(ctx)

	for svcID, p := range i.proxies {
		cb := proxy.Callbacks{
			ConnDelta: func(delta int) {
				// The order of operations is important here.
				p.lastActivityTime.Store(time.Now())
				p.activeConns.Add(int32(delta))
			},
			Ready: func(ctx context.Context, wait bool) (ready bool, hostName string) {

				// If we are ready or asked not to wait, return immediately.
				st := i.lastHostState.Load().(host.State)
				lastPing := p.lastPing.Load()
				ready = lastPing && st.Status == host.StatusStarted
				if ready || !wait {
					return ready, st.HostName
				}

				// This ensures if the context is cancelled, we stop waiting
				// and return an error.
				stop := context.AfterFunc(ctx, func() {
					i.hostReadyCond.L.Lock()
					defer i.hostReadyCond.L.Unlock()
					i.hostReadyCond.Broadcast()
				})
				defer stop()

				i.hostReadyCond.L.Lock()
				defer i.hostReadyCond.L.Unlock()

				for {
					select {
					case i.wakupCh <- struct{}{}:
					}
					i.hostReadyCond.Wait()
					if ctx.Err() != nil {
						return false, ""
					}
					st := i.lastHostState.Load().(host.State)
					lastPing := p.lastPing.Load()
					ready = lastPing && st.Status == host.StatusStarted
					if ready {
						return true, st.HostName
					}
				}

			},
		}
		go cancel(p.p.Run(innerCtx, cb))
	}

	<-innerCtx.Done()
	return context.Cause(innerCtx)
}

func (i *instance) runReconLoop(ctx context.Context) error {

	const timerInterval = 5 * time.Second

	var lastActivityTime atomic.Value
	lastActivityTime.Store(time.Now())
	var connCount atomic.Int32
	go func() {
		for d := range connDeltaCh {
			connCount.Add(int32(d))
			lastActivityTime.Store(time.Now())
		}
	}()

	var wakeupRequested bool
	timer := time.NewTicker(timerInterval)
	var ctxCancel context.CancelFunc
	var ctx context.Context
	for {
		if ctxCancel != nil {
			ctxCancel()
		}
		// Wait for either 5 seconds or a wakeup signal.
		select {
		case <-timer.C:
		case <-wakeupEC2Ch:
			wakeupRequested = true
		}
		ctx, ctxCancel = context.WithTimeout(context.Background(), timerInterval-time.Second)
		details, err := getEC2Details(ctx, cfgInst.EC2.InstanceID)
		if err != nil {
			log.Printf("error getting EC2 instance details: %v", err)
			continue
		}
		ec2IPAddress.Store(details.IP)

		log.Printf("ec2-recon-loop: instance %s is in state %s with IP %s, active connections: %d, last activity: %s",
			cfgInst.EC2.InstanceID, details.State, details.IP, connCount.Load(), lastActivityTime.Load().(time.Time).Format(time.RFC3339))

		// Decision

		idle := connCount.Load() == 0 && time.Since(lastActivityTime.Load().(time.Time)) > cfgInst.Timeout.Duration

		if details.State != ec2types.InstanceStateNameRunning {
			ec2CurStatus.Store(ec2StatusNotReady)
		}

		if wakeupRequested && details.State == ec2types.InstanceStateNameStopped {
			log.Printf("waking up EC2 instance %s", cfgInst.EC2.InstanceID)
			// TODO host start

		} else if idle && details.State == ec2types.InstanceStateNameRunning {
			ec2CurStatus.Store(ec2StatusNotReady)
			log.Printf("stopping EC2 instance %s due to inactivity", cfgInst.EC2.InstanceID)
			// TODO host stop

		} else if details.State == ec2types.InstanceStateNameRunning {
			u := fmt.Sprintf("http://%s:%d/", details.IP, cfgInst.Services[0].HTTP.ServicePort)
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				log.Printf("health-check: request to %s failed: %v", u, err)
				ec2CurStatus.Store(ec2StatusNotReady)
				continue
			}
			resp.Body.Close()
			if resp.StatusCode < 200 || resp.StatusCode >= 500 {
				log.Printf("health-check: request to %s returned status %d", u, resp.StatusCode)
				ec2CurStatus.Store(ec2StatusNotReady)
				continue
			}
			log.Printf("health-check: EC2 instance %s is healthy", cfgInst.EC2.InstanceID)
			ec2CurStatus.Store(ec2StatusReady)
			ec2ReadyCond.Broadcast()
			wakeupRequested = false
		}

	}
}

func run(configPath string) error {
	cfg, err := config.LoadFile(configPath)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	instances := make(map[string]*instance, len(cfg.Instances))
	for i, instCfg := range cfg.Instances {
		var err error
		instances[i], err = newInstance(instCfg)
		if err != nil {
			return fmt.Errorf("instance %s: %w", i, err)
		}

	}
	wg := sync.WaitGroup{}
	wg.Add(len(instances))

	return err
}

func main() {
	log.SetFlags(0)
	configPath := flag.String("config", "zipnap.yaml", "Path to the configuration file")
	flag.Parse()
	if err := run(*configPath); err != nil {
		log.Fatal(err)
	}
}
