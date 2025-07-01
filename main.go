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
	cfg := `{
		"instances":[
      {
        "name":"hi",
        "type": "ec2",
        "ec2": {
        },
        "timeout": "4m"
      }
    ]
}
`
	_, err := config.Load(strings.NewReader(cfg))
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
