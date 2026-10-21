package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/greymass/creationindex/internal/api"
	"github.com/greymass/creationindex/internal/store"
	"github.com/greymass/roborovski/libraries/config"
	"github.com/greymass/roborovski/libraries/logger"
	"github.com/greymass/roborovski/libraries/server"
)

var Version = "dev"

var categories = []string{"startup", "sync", "status", "http"}

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
	logger.Printf("startup", "  db-path: %s", cfg.DBPath)
	logger.Printf("startup", "  actionstream-source: %s", cfg.ActionStreamSource)
	logger.Printf("startup", "  program-account: %s", cfg.ProgramAccount)
	logger.Printf("startup", "  http-listen: %s", cfg.HTTPListen)
	logger.Printf("startup", "  http-socket: %s", cfg.HTTPSocket)

	db, err := store.Open(cfg.DBPath)
	if err != nil {
		logger.Fatal("%v", err)
	}

	eosio := newEosioSyncer(db, cfg)
	if err := eosio.Start(); err != nil {
		logger.Fatal("Failed to start eosio syncer: %v", err)
	}

	var giftSyncer *GiftSyncer
	if cfg.ProgramAccount == "" {
		logger.Printf("startup", "program-account is not set, gift stream disabled")
	} else {
		giftSyncer = NewGiftSyncer(cfg, db)
		if err := giftSyncer.Start(); err != nil {
			logger.Fatal("Failed to start gift syncer: %v", err)
		}
	}

	var servers []*http.Server
	for _, addr := range []string{cfg.HTTPListen, cfg.HTTPSocket} {
		if addr == "none" || addr == "" {
			continue
		}
		srv := &http.Server{Handler: api.New(db, cfg.ProgramAccount)}
		go srv.Serve(server.SocketListen(addr))
		servers = append(servers, srv)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	logger.Printf("startup", "Shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, srv := range servers {
		srv.Shutdown(ctx)
	}
	eosio.Stop()
	if giftSyncer != nil {
		giftSyncer.Stop()
	}
	if err := db.Close(); err != nil {
		logger.Printf("startup", "Failed to close database: %v", err)
	}
	logger.Printf("startup", "Shutdown complete")
}
