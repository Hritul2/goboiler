package main

import (
	"log"

	"github.com/hritul2/goboiler/internal/utils/config"
)

func main() {
	// configPath := "configs/config.json"
	cfg, err := config.LoadConfig(nil)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}
	_ = cfg
}
