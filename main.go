package main

import (
	_ "embed"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/mathspace/zipnap/config"
)

type ec2Status string

const (
	ec2StatusReady  ec2Status = "ready"
	ec2StatusUp     ec2Status = "up"
	ec2StatusWaking ec2Status = "waking"
	ec2StatusDown   ec2Status = "down"
)

var (
	configPath string

	cfgInst config.Instance

	// activeRequests is the number of active requests in flight.
	// It's updated on connection and disconnection.
	activeRequests atomic.Int32

	// wakeupEC2Ch is a channel used to signal that the EC2 instance should be
	// woken up.
	wakeupEC2Ch = make(chan struct{}, 1)

	// ec2CurStatus is the current status of the EC2 instance.
	ec2CurStatus atomic.Value
	ec2ReadyCond = sync.NewCond(&sync.Mutex{})

	ec2IPAddress atomic.Value
)

//go:embed waiting.html
var waitingPageBytes []byte

func httpHandler(w http.ResponseWriter, r *http.Request) {
	activeRequests.Add(1)
	defer activeRequests.Add(-1)

	// Show waiting page OR block the response until EC2 is ready.

	status := ec2CurStatus.Load().(ec2Status)
	if status != ec2StatusReady {

		if status == ec2StatusDown {
			// Nudge the EC2 instance to wake up if it's not ready, without
			// blocking.
			select {
			case wakeupEC2Ch <- struct{}{}:
			default:
			}
		}

		if cfgInst.Services[0].HTTP.ShowWaitingPage {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write(waitingPageBytes)
			return
		}
		ec2ReadyCond.L.Lock()
		for ec2CurStatus.Load().(ec2Status) != ec2StatusReady {
			ec2ReadyCond.Wait()
		}
		ec2ReadyCond.L.Unlock()
	}

	// Proxy the request to the EC2 instance.

	host := ec2IPAddress.Load().(string)
	if cfgInst.Services[0].HTTP.ServicePort != 80 {
		host += strconv.Itoa(cfgInst.Services[0].HTTP.ServicePort)
	}
	u := &url.URL{
		Scheme: "http",
		Host:   host,
	}
	reverseProxy := httputil.NewSingleHostReverseProxy(u)
	reverseProxy.ServeHTTP(w, r)
}

func run() error {
	cfg, err := config.LoadFile(configPath)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	cfgInst = cfg.Instances[0]

	ec2CurStatus.Store(ec2StatusDown)

	http.HandleFunc("/", httpHandler)
	http.ListenAndServe(fmt.Sprintf(":%d", cfgInst.Services[0].HTTP.ProxyPort), nil)

	return err
}

func main() {
	log.SetFlags(0)
	flag.StringVar(&configPath, "config", "zipnap.yaml", "Path to the configuration file")
	flag.Parse()
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
