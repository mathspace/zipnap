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

	"github.com/mathspace/zipnap/activator"
	"github.com/mathspace/zipnap/activator/httpproxy"
	"github.com/mathspace/zipnap/activator/schedule"
	"github.com/mathspace/zipnap/config"
	"github.com/mathspace/zipnap/host"
	"github.com/mathspace/zipnap/host/ec2host"
)

type instanceRuntime struct {
	id            string
	cfg           config.Instance
	activators    map[string]activator.Activator
	host          host.Host    // Host interface for managing the EC2 instance
	lastHostState atomic.Value // host.State
	wakupCh       chan struct{}
	hostReadyCond *sync.Cond // Condition variable to signal when the host is ready
	logger        *log.Logger
	wakeLocks     atomic.Uint32
}

func newInstance(ctx context.Context, id string, cfg config.Instance) (*instanceRuntime, error) {

	activators := make(map[string]activator.Activator, len(cfg.Activators))

	for actID, act := range cfg.Activators {
		logger := log.New(os.Stdout, fmt.Sprintf("instance[%s] activator[%s]: ", actID), 0)
		var a activator.Activator
		if act.HTTPProxy != nil {
			a = httpproxy.New(act.HTTPProxy, actID, logger)
		} else if act.TCPProxy != nil {
			panic("TCPProxy not implemented yet")
		} else if act.Schedule != nil {
			a = schedule.New(act.Schedule, logger)
		} else {
			panic("unreachable: activator type not set")
		}
		activators[actID] = a
	}

	logger := log.New(os.Stdout, fmt.Sprintf("instance[%s]: ", id), 0)
	var h host.Host
	var err error
	if cfg.EC2 != nil {
		h, err = ec2host.New(ctx, cfg.EC2, logger)
		if err != nil {
			return nil, err
		}
	} else {
		panic("unreachable: invalid host type")
	}

	inst := &instanceRuntime{
		id:            id,
		cfg:           cfg,
		activators:    activators,
		wakupCh:       make(chan struct{}, 1),
		host:          h,
		hostReadyCond: sync.NewCond(&sync.Mutex{}),
	}
	inst.lastHostState.Store(host.State{Status: host.StatusUnknown})
	return inst, nil
}

func (i *instanceRuntime) hostStateCallback() host.State {
	return i.lastHostState.Load().(host.State)
}

func (i *instanceRuntime) wakeLockCallback(ctx context.Context, wake bool) (unlock func(), err error) {
	i.hostReadyCond.L.Lock()
	defer i.hostReadyCond.L.Unlock()
	context.AfterFunc(ctx, func() {
		i.hostReadyCond.L.Lock()
		defer i.hostReadyCond.L.Unlock()
		i.hostReadyCond.Broadcast()
	})

	for i.lastHostState.Load().(host.State).Status != host.StatusStarted {
		if wake {
			select {
			case i.wakupCh <- struct{}{}:
			default:
				// If the channel is already full, we don't need to wake up again.
			}
		}
		i.hostReadyCond.Wait()
	}
	
	return
}

func (i *instanceRuntime) runActivators(ctx context.Context) error {
	innerCtx, cancel := context.WithCancelCause(ctx)

	for _, a := range i.activators {
		a.RegisterCallbacks(activator.Callbacks{
			HostState: i.hostStateCallback,
			WakeLock:  i.wakeLockCallback,
		})
		go cancel(a.Run(innerCtx))
	}

	<-innerCtx.Done()
	return context.Cause(innerCtx)
}

func (i *instanceRuntime) runReconLoop(ctx context.Context) {

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
		for _, p := range i.activators {
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
	instances := make(map[string]*instanceRuntime, len(cfg.Instances))
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
