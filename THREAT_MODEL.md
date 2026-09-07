# zipnap threat model

## Overview

Zipnap is an operator-configured Go daemon that starts/stops AWS hosts in response to activity and idle timeouts. The actual main path constructs EC2 or RDS hosts and HTTP-proxy or schedule activators. HTTP requests acquire wake locks, wait for health, and proxy to the host address returned by AWS; a waiting-page/SSE endpoint handles startup delay. TCP proxy and GitHub-runner components exist in source, but main explicitly panics for TCP and does not construct the GHA host choice accepted by configuration. Their component capabilities are therefore separate from the functioning daemon path.

| Component | Source |
| --- | --- |
| Configuration loading | config/config.go:137 |
| Actual component construction | main.go:40 |
| HTTP wake/proxy | activator/httpproxy/httpproxy.go:137 |
| AWS EC2 host | host/ec2host/ec2.go:26 |
| AWS RDS host | host/rdshost/rds.go:24 |

| Deployment or workflow | Resource or capability | Configuration and precedence | Safe effective value or location | Readers, writers, or recipients | Enforcing control | Evidence or unknowns |
| --- | --- | --- | --- | --- | --- | --- |
| Daemon config | Lifecycle targets | --config or zipnap.yaml; configuration validates one host type and positive timeout | &lt;cwd&gt;/zipnap.yaml by default; configured EC2/RDS instance identifier | Operator file writer and AWS SDK under default credential chain | File permissions, config validation and AWS IAM | main.go:291 |
| EC2 | Lifecycle and destination | Configured instance_id; AWS DescribeInstances returns PrivateIpAddress | Start/hibernate-stop named instance; proxy targets its private IP plus configured host_port | AWS EC2 API, daemon and backend peers | IAM resource scope; clients cannot choose instance_id per request | host/ec2host/ec2.go:39 |
| RDS | Lifecycle and destination | Configured instance_id; DescribeDBInstances endpoint | Start/stop named database; reported RDS endpoint plus configured port | AWS RDS API and daemon | IAM and config; protocol suitability remains operator obligation | host/rdshost/rds.go:37 |
| HTTP proxy | Listener and forwarding | listen_addr optional and listen_port required; empty address retained | :&lt;listen_port&gt; when address omitted; HTTP backend at state.Addr:host_port | Any admitted network peer and configured backend | Host ingress; handler has no authentication; timeouts/wake locks manage lifecycle | activator/httpproxy/httpproxy.go:209 |
| HTTP health | Outbound probe | Configured health path/default slash, interval/default 3s, expected code/default 200 | http://&lt;AWS-reported-host&gt;:&lt;host_port&gt;&lt;health-path&gt; | Daemon and backend; default HTTP client handles response | Context deadline; destination comes from operator/AWS state | activator/httpproxy/httpproxy.go:64 |
| Unwired components | TCP and GitHub polling capability | Component constructors exist; main TCP path panics and supports no GHA host | Not reachable through a successful current daemon startup using these choices | Only a separate direct component caller could activate them | Explicit startup failure, not a network security gate | main.go:49 |

## Threat Model, Trust Boundaries, and Assumptions

Protect intended cloud instance identity, lifecycle availability, cloud cost, internal backend exposure, and daemon credentials. A reachable HTTP client can wake and use a configured backend but does not directly choose an AWS instance identifier. The operator controls configuration and AWS credential selection; IAM is the actual cloud authorization layer. Proxy admission must be supplied by network controls or an integrating authentication layer if the backend is not public. Host/port configuration is privileged network-destination authority. A health probe is availability evidence, not user authentication. Schedule activation is operator intent and is distinct from caller-triggered traffic.

This model uses the repository’s own source and generic host/caller obligations. No private deployment facts, observed exploitation, or inferred tenant relationships are included. Source review establishes the operations below; it does not establish every dependency’s implementation or the permissions of an actual installation. The operator must distinguish a deliberately granted capability from a lower-trust input gaining a new one.

AWS account/IAM, concrete configured IDs and ports, backend authentication and ingress are deployment facts absent from this reusable model. config/config.go:87 accepts GHA as a host type but main.go:62 constructs only EC2/RDS; TCP similarly validates yet panics at main.go:50. The standalone GHA constructor reads an operator-selected token environment variable at activator/gharunner/gharunner.go:30; current main grants it no runtime path.

## Attack Surface, Mitigations, and Attacker Stories

These are prioritized hypotheses for investigation, not validated findings. Priority reflects plausible capability gain; each prerequisite must hold before assigning a deployment-specific severity.

| Priority | Scenario and capability gain | Prerequisites | Impact | Existing controls | Mitigation | Evidence |
| --- | --- | --- | --- | --- | --- | --- |
| P1 | An unauthenticated peer reaches a private backend through the proxy and wakes costly infrastructure. | Proxy published to a broader audience than backend policy permits. | Unauthorized backend reachability and spend. | Fixed AWS-derived destination; backend may enforce its own authentication. | Restrict proxy ingress and require admission before acquiring wake locks. | activator/httpproxy/httpproxy.go:146 |
| P2 | Repeated or long-lived requests keep a host awake indefinitely or exhaust waiting resources. | Reachable proxy and insufficient upstream connection/budget limits. | Cloud cost and service availability impact. | Request context, waiting timeout and deferred unlock exist; idle timeout cannot bound continuous legitimate-looking traffic. | Bound concurrent waits/connections and apply explicit lifecycle spending limits. | activator/httpproxy/httpproxy.go:144 |
| P2 | A health endpoint redirects probes to an unintended destination. | Attacker controls configured backend response; default client follows allowed redirect behavior. | Conditional internal network probing; no response exfiltration path established. | Initial destination is AWS-derived; health result is reduced to availability state. | Disable redirects for health checks or constrain final probe destinations. | activator/httpproxy/httpproxy.go:66 |
| P2 | Wrong operator configuration stops an unintended valuable instance. | Misconfiguration and IAM permitting that target. | Backend outage. | Instance IDs fixed in validated configuration; IAM bounds permissible resources. | Narrow IAM to designated disposable hosts and verify identity before enabling lifecycle management. | host/rdshost/rds.go:47 |

## Severity Calibration (Critical, High, Medium, Low)

Critical requires demonstrated reach into highly consequential infrastructure, not simply AWS SDK usage.

High fits exposure of a privileged internal backend or material unintended lifecycle outage with actual cloud permissions.

Medium fits sustained cloud-cost abuse, bounded proxy exhaustion, or verified health-probe misuse.

Low fits invalid config startup failure or a local waiting-page issue. TCP/GHA source presence does not establish exposed services through main.

Confidence in the source-described data flow is separate from confidence in exploitability. A finding requires a concrete lower-trust entry, an effective control failure, and a consequential new capability. Host compromise assumed at the outset, deliberate operator authority, and self-only errors do not supply that missing evidence.

Repository: github.com/mathspace/zipnap

Version: ce69f49359bbd9d4eaad705e01a1d23f6fd1e9d0
