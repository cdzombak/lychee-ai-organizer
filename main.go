package main

import (
	"flag"
	"fmt"
	"log"
	"os"
)

var (
	version string = "<dev>"
)

func main() {
	var configPath = flag.String("config", "config.json", "Path to configuration file")
	var showVersion = flag.Bool("version", false, "Print version information and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("lychee-ai-organizer %s\n", version)
		os.Exit(0)
	}

	if *configPath == "" {
		log.Fatal("Config file path is required (-config)")
	}

	app := NewApp(*configPath)
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
