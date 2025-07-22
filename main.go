package main

import (
	"context"
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"log"
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
	"github.com/mathspace/zipnap/instance"
)

type instanceRuntime struct {
	id            string
	cfg           *config.Instance
	activators    map[string]activator.Activator
	callbacks     activator.Callbacks // Callbacks for activators to use
	host          host.Host           // Host interface for managing the EC2 instance
	wakeupCh      chan struct{}       // Channel to signal wakeup requests
	hostReadyCond *sync.Cond          // Condition variable to signal when the host is ready
	logger        *log.Logger         // Logger prefixed with instance ID
	lockDeltaCh   chan int            // Channel to signal changes in the number of wake locks
	state         atomic.Value        // activator.InstanceState
}

func newInstanceRuntime(ctx context.Context, id string, cfg *config.Instance) (*instanceRuntime, error) {

	activators := make(map[string]activator.Activator, len(cfg.Activators))

	for actID, act := range cfg.Activators {
		logger := log.New(os.Stdout, fmt.Sprintf("instance[%s] activator[%s]: ", id, actID), 0)
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
		wakeupCh:      make(chan struct{}, 1),
		host:          h,
		hostReadyCond: sync.NewCond(&sync.Mutex{}),
		logger:        logger,
	}
	inst.callbacks = activator.Callbacks{
		State:       inst.hostStateCallback,
		HealthyLock: inst.wakeLockCallback,
	}
	inst.state.Store(instance.UnhealthyState)
	return inst, nil
}

// hostStateCallback is called by activators to get the current state of the
// host.
func (i *instanceRuntime) hostStateCallback() instance.State {
	return i.state.Load().(instance.State)
}

// wakeLockCallback is called by activators to request a wake lock on the host.
func (i *instanceRuntime) wakeLockCallback(ctx context.Context, wake bool) (unlock func(), err error) {
	i.hostReadyCond.L.Lock()
	defer i.hostReadyCond.L.Unlock()
	context.AfterFunc(ctx, func() {
		i.hostReadyCond.L.Lock()
		defer i.hostReadyCond.L.Unlock()
		i.hostReadyCond.Broadcast()
	})

	for !i.state.Load().(instance.State).Healthy {
		if wake {
			select {
			case i.wakeupCh <- struct{}{}:
			default:
				// If the channel is already full, we don't need to wake up again.
			}
		}
		i.hostReadyCond.Wait()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}

	i.lockDeltaCh <- 1
	return func() { i.lockDeltaCh <- -1 }, nil
}

func (i *instanceRuntime) runActivators(ctx context.Context) error {
	innerCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)

	for _, a := range i.activators {
		a.RegisterCallbacks(i.callbacks)
		go cancel(a.Run(innerCtx))
	}

	<-innerCtx.Done()
	return context.Cause(innerCtx)
}

func (i *instanceRuntime) runReconLoop(ctx context.Context) {

	logger := log.New(i.logger.Writer(), fmt.Sprintf("instance[%s] recon: ", i.id), 0)

	var idle atomic.Bool

	// idle determination loop.
	go func() {
		var lastUnlock time.Time
		wakeLocks := 0
		for {
			select {
			case delta := <-i.lockDeltaCh:
				if delta < 0 {
					lastUnlock = time.Now()
				}
				wakeLocks += delta
				idle.Store(wakeLocks == 0 && time.Since(lastUnlock) <= i.cfg.Timeout.Duration)
			case <-ctx.Done():
				return
			}
		}
	}()

	var wakeupRequested bool
	for ctx.Err() == nil {
		ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		if err := func() error {

			logger.Printf("waking up")

			hostSt, err := i.host.State(ctx)
			if err != nil {
				return err
			}

			logger.Printf("host state: %s", hostSt.Status)
			idle := idle.Load()

			if wakeupRequested && hostSt.Status == host.StatusStopped {
				i.state.Store(instance.UnhealthyState)
				log.Print("waking up host")
				if err := i.host.Start(ctx); err != nil {
					return err
				}
			} else if hostSt.Status == host.StatusStarted {
				if wakeupRequested || !idle {
					i.state.Store(instance.State{
						Healthy: true,
						Addr:    hostSt.Addr,
					})
				}
				if wakeupRequested {
					i.hostReadyCond.Broadcast()
					wakeupRequested = false
				} else if idle {
					i.state.Store(instance.UnhealthyState)
					log.Print("stopping due to inactivity")
					if err := i.host.Stop(ctx); err != nil {
						return err
					}
				}
			} else {
				i.state.Store(instance.UnhealthyState)
			}

			return nil
		}(); err != nil {
			if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
				logger.Printf("error: %v", err)
			}
		}

		<-ctx.Done()
		cancel()
		select {
		case <-i.wakeupCh:
			wakeupRequested = true
		default:
		}
	}
}

func run(configPath string) error {
	cfg, err := config.LoadFile(configPath)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)

	instanceRuntimes := make(map[string]*instanceRuntime, len(cfg.Instances))
	for i, instCfg := range cfg.Instances {
		var err error
		instanceRuntimes[i], err = newInstanceRuntime(ctx, i, instCfg)
		if err != nil {
			return fmt.Errorf("instance[%s]: %w", i, err)
		}
	}

	wg := sync.WaitGroup{}
	for _, inst := range instanceRuntimes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			inst.logger.Print("starting activators")
			cancel(inst.runActivators(ctx))
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			inst.logger.Print("starting recon loop")
			inst.runReconLoop(ctx)
		}()
	}

	// TODO add signal handling to cancel the context on SIGINT/SIGTERM

	wg.Wait()
	return context.Cause(ctx)
}

func main() {
	log.SetFlags(0)
	configPath := flag.String("config", "zipnap.yaml", "Path to the configuration file")
	flag.Parse()
	if err := run(*configPath); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			log.Println("terminated")
		} else {
			log.Fatal(err)
		}
	}
}
