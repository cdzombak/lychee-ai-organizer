package main

import (
	"flag"
	"log"
)

func main() {
	var configPath = flag.String("config", "", "Path to configuration file")
	var cachePath = flag.String("cache", "", "Path to cache file")
	flag.Parse()

	if *configPath == "" {
		log.Fatal("Config file path is required (-config)")
	}

	if *cachePath == "" {
		log.Fatal("Cache file path is required (-cache)")
	}

	app := NewApp(*configPath, *cachePath)
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}