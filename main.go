package main

import (
	"flag"
	"log"
)

func main() {
	var configPath = flag.String("config", "config.json", "Path to configuration file")
	flag.Parse()

	if *configPath == "" {
		log.Fatal("Config file path is required (-config)")
	}

	app := NewApp(*configPath)
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
