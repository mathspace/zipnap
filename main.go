package main

import (
	"flag"
	"log"
	"strings"

	"github.com/mathspace/zipnap/config"
)

var (
	configPath string
)

func run() error {
	// Load the configuration file
	cfg := `
instances:
  - name: example-instance
    type: ec2
    ec2:
      instance_id: i-1234567890abcdef0
`
	config.Load(strings.NewReader(cfg))
	return nil
}

func main() {
	log.SetFlags(0)
	flag.StringVar(&configPath, "config", "zipnap.yaml", "Path to the configuration file")
	flag.Parse()
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
