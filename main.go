package main

import (
	"context"
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/mathspace/zipnap/config"
	"github.com/mathspace/zipnap/host"
	"github.com/mathspace/zipnap/host/ec2host"
	"github.com/mathspace/zipnap/proxy"
	"github.com/mathspace/zipnap/proxy/httpproxy"
)

type instanceProxy struct {
	p                proxy.Proxy
	ready            atomic.Bool
	activeConns      atomic.Int32
	lastActivityTime atomic.Value // time.Time
	logger           *log.Logger
}

type instance struct {
	id            string
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

	proxies := make(map[string]*instanceProxy, len(cfg.Activators))
	for svcID, svc := range cfg.Activators {
		logger := log.New(os.Stdout, fmt.Sprintf("instance[%s] proxy[%s]: ", svcID), 0)
		var p proxy.Proxy
		switch svc.Type {
		case config.ServiceTypeHTTP:
			p = httpproxy.New(svc, logger)
		case config.ServiceTypeTCP:
			return nil, fmt.Errorf("tcp proxy not implemented yet")
		default:
			panic("unreachable")
		}
		proxies[svcID] = &instanceProxy{
			p:      p,
			logger: logger,
		}
	}

	logger := log.New(os.Stdout, fmt.Sprintf("instance[%s]: ", id), 0)
	var h host.Host
	var err error
	switch cfg.Type {
	case config.InstanceTypeEC2:
		h, err = ec2host.New(ctx, cfg, logger)
		if err != nil {
			return nil, err
		}
	default:
		panic("unreachable")
	}

	inst := &instance{
		id:            id,
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

	for _, p := range i.proxies {
		cb := proxy.Callbacks{
			ConnDelta: func(delta int) {
				// The order of operations is important here.
				p.lastActivityTime.Store(time.Now())
				p.activeConns.Add(int32(delta))
			},
			WaitHostReady: func(ctx context.Context, wait bool) (ready bool, hostName string) {

				// If we are ready or asked not to wait, return immediately.
				st := i.lastHostState.Load().(host.State)
				if p.ready.Load() || !wait {
					return ready, st.Addr
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
					if p.ready.Load() {
						return true, st.Addr
					}
				}

			},
		}
		go cancel(p.p.Run(innerCtx, cb))
	}

	<-innerCtx.Done()
	return context.Cause(innerCtx)
}

func (i *instance) runReconLoop(ctx context.Context) {

	logger := log.New(i.logger.Writer(), fmt.Sprintf("instance[%s] recon: ", i.id), 0)

	const timerInterval = 5 * time.Second
	var wakeupRequested bool
	timer := time.NewTicker(timerInterval)

	for {

		// Wait for either 5 seconds or a wakeup signal.
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		case <-i.wakupCh:
			wakeupRequested = true
		}

		logger.Printf("waking up")

		st, err := i.host.State(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return
			}
			logger.Printf("error getting host state: %v", err)
			continue
		}
		i.lastHostState.Store(st)

		logger.Printf("host state: %s", st.Status)

		// Decision

		idle := true
		for _, p := range i.proxies {
			if p.activeConns.Load() > 0 {
				idle = false
				break
			}
			if time.Since(p.lastActivityTime.Load().(time.Time)) <= i.cfg.Timeout.Duration {
				idle = false
				break
			}
		}

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
