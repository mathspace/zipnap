package main

import (
	"context"
	_ "embed"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"github.com/mathspace/zipnap/config"
)

type ec2Status string

const (
	ec2StatusReady    ec2Status = "ready"
	ec2StatusNotReady ec2Status = "not_ready"
)

var (
	configPath string

	cfgInst config.Instance

	// connDeltaCh is a channel used to signal changes in the number of active
	// requests.
	connDeltaCh = make(chan int, 1)

	// wakeupEC2Ch is a channel used to signal that the EC2 instance should be
	// woken up.
	wakeupEC2Ch = make(chan struct{}, 1)

	// ec2CurStatus is the current status of the EC2 instance.
	ec2CurStatus atomic.Value
	ec2ReadyCond = sync.NewCond(&sync.Mutex{})

	ec2IPAddress atomic.Value

	ec2Client *ec2.Client
)

var (
	//go:embed waiting.html
	waitingPageBytes []byte

	waitingPageTpl = template.Must(template.New("").Parse(string(waitingPageBytes)))
)

// httpHandler handles incoming HTTP requests and proxies them to HTTP service.
func httpHandler(w http.ResponseWriter, r *http.Request) {
	connDeltaCh <- 1
	defer func() {
		connDeltaCh <- -1
	}()

	// Show waiting page OR block the response until EC2 is ready.

	status := ec2CurStatus.Load().(ec2Status)
	if status != ec2StatusReady {

		if status == ec2StatusNotReady {
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
			waitingPageTpl.Execute(w, map[string]any{
				"Name": cfgInst.Services[0].Name,
			})
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
		host += ":" + strconv.Itoa(cfgInst.Services[0].HTTP.ServicePort)
	}
	u := &url.URL{
		Scheme: "http",
		Host:   host,
	}
	reverseProxy := httputil.NewSingleHostReverseProxy(u)
	reverseProxy.ServeHTTP(w, r)
}

type ec2InstanceDetails struct {
	IP    string
	State ec2types.InstanceStateName
}

func getEC2Client(ctx context.Context) (*ec2.Client, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	return ec2.NewFromConfig(awsCfg), nil
}

func getEC2Details(ctx context.Context, instanceID string) (ec2InstanceDetails, error) {
	ret := ec2InstanceDetails{}

	// Get the current status of the EC2 instance.
	out, err := ec2Client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		InstanceIds: []string{instanceID},
	})
	if err != nil {
		return ret, err
	}
	for _, inst := range out.Reservations[0].Instances {
		if inst.State == nil {
			ret.State = ""
		} else {
			ret.State = inst.State.Name
		}
		if inst.PrivateIpAddress != nil {
			ret.IP = *inst.PrivateIpAddress
		} else {
			ret.IP = ""
		}
	}
	return ret, nil
}

func ec2ReconciliationLoop() {
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
			if _, err := ec2Client.StartInstances(ctx, &ec2.StartInstancesInput{
				InstanceIds: []string{cfgInst.EC2.InstanceID},
			}); err != nil {
				log.Printf("error starting EC2 instance %s: %v", cfgInst.EC2.InstanceID, err)
			}

		} else if idle && details.State == ec2types.InstanceStateNameRunning {
			ec2CurStatus.Store(ec2StatusNotReady)
			log.Printf("stopping EC2 instance %s due to inactivity", cfgInst.EC2.InstanceID)
			if _, err := ec2Client.StopInstances(ctx, &ec2.StopInstancesInput{
				InstanceIds: []string{cfgInst.EC2.InstanceID},
				Hibernate:   aws.Bool(true),
			}); err != nil {
				log.Printf("error stopping EC2 instance %s: %v", cfgInst.EC2.InstanceID, err)
			}

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

func run() error {
	cfg, err := config.LoadFile(configPath)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	cfgInst = cfg.Instances[0]

	ec2CurStatus.Store(ec2StatusNotReady)

	ec2Client, err = getEC2Client(context.Background())
	if err != nil {
		return fmt.Errorf("failed to create EC2 client: %w", err)
	}

	go ec2ReconciliationLoop()

	http.HandleFunc("/", httpHandler)

	log.Printf("starting http proxy on port %d", cfgInst.Services[0].HTTP.ProxyPort)
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
