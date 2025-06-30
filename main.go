package main

import (
	"flag"
	"log"

	"github.com/mathspace/zipnap/config"
)

var (
	configPath string
)

func run() error {
	// Load the configuration file
	config.Load(nil)
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
