package main

import (
	"fmt"
	"log"

	"github.com/1729prashant/blog-aggregator/internal/config"
)

func main() {
	// Read the config file
	cfg, err := config.Read()
	if err != nil {
		log.Fatalf("Failed to read config: %v", err)
	}
	fmt.Printf("Initial Config: %+v\n", cfg)

	// Set the current user and update the config file
	err = cfg.SetUser("Prashant")
	if err != nil {
		log.Fatalf("Failed to update user in config: %v", err)
	}

	// Re-read the config file and print its contents
	cfg, err = config.Read()
	if err != nil {
		log.Fatalf("Failed to read config: %v", err)
	}
	fmt.Printf("Updated Config: %+v\n", cfg)

}
