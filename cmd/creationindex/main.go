package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/greymass/roborovski/libraries/config"
	"github.com/greymass/roborovski/libraries/logger"
)

var Version = "dev"

var categories = []string{"startup", "sync", "status"}

func main() {
	cfg := &Config{}
	if err := config.Load(cfg, os.Args[1:]); err != nil {
		logger.Fatal("Config error: %v", err)
	}

	logger.RegisterCategories(categories...)
	if cfg.Debug {
		logger.SetMinLevel(logger.LevelDebug)
		logger.SetCategoryFilter(nil)
	} else {
		logger.SetCategoryFilter(cfg.LogFilter)
	}

	if cfg.LogFile != "" {
		if err := logger.SetLogFile(cfg.LogFile); err != nil {
			logger.Fatal("Failed to open log file %s: %v", cfg.LogFile, err)
		}
		defer logger.Close()
	}

	logger.Printf("startup", "creationindex %s starting", Version)
	logger.Printf("startup", "  actionstream-source: %s", cfg.ActionStreamSource)

	syncer := NewSyncer(cfg)
	if err := syncer.Start(); err != nil {
		logger.Fatal("Failed to start syncer: %v", err)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	logger.Printf("startup", "Shutting down...")
	syncer.Stop()
	logger.Printf("startup", "Shutdown complete")
}
