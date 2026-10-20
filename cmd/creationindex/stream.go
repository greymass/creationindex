package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/greymass/roborovski/libraries/actionstream"
	"github.com/greymass/roborovski/libraries/chain"
	"github.com/greymass/roborovski/libraries/logger"
)

func streamAddress(source string) (string, error) {
	if !strings.HasPrefix(source, "stream://") {
		return "", fmt.Errorf("invalid actionstream-source %q: must be stream://host:port or stream:///path/to.sock", source)
	}
	addr := strings.TrimPrefix(source, "stream://")
	if addr == "" {
		return "", fmt.Errorf("actionstream-source is required")
	}
	return addr, nil
}

func creationFilter() actionstream.Filter {
	return actionstream.Filter{
		Contracts: []uint64{chain.StringToName("eosio")},
		Actions:   []uint64{chain.StringToName("newaccount")},
	}
}

type Syncer struct {
	cfg    *Config
	client *actionstream.Client
	stopCh chan struct{}
	done   chan struct{}
}

func NewSyncer(cfg *Config) *Syncer {
	return &Syncer{cfg: cfg, stopCh: make(chan struct{}), done: make(chan struct{})}
}

func (s *Syncer) Start() error {
	addr, err := streamAddress(s.cfg.ActionStreamSource)
	if err != nil {
		return err
	}

	clientCfg := actionstream.DefaultClientConfig()
	clientCfg.Debug = s.cfg.Debug
	clientCfg.Decode = true

	s.client = actionstream.NewClient(addr, creationFilter(), 1, clientCfg)
	if err := s.client.Connect(); err != nil {
		return err
	}
	logger.Printf("sync", "Connected to actionstream at %s, starting from seq 1", addr)

	go s.loop()
	return nil
}

func (s *Syncer) Stop() {
	close(s.stopCh)
	if s.client != nil {
		s.client.Close()
	}
	<-s.done
}

func (s *Syncer) loop() {
	defer close(s.done)

	statusTicker := time.NewTicker(10 * time.Second)
	defer statusTicker.Stop()

	var received, lastSeq uint64
	for {
		select {
		case <-s.stopCh:
			return
		case <-statusTicker.C:
			logger.Printf("status", "Status: actions_received=%d last_seq=%d head=%d lib=%d catchup=%v",
				received, lastSeq, s.client.GetHeadSeq(), s.client.GetLIBSeq(), s.client.IsCatchupComplete())
		default:
			action, ok, err := s.client.NextWithTimeout(100 * time.Millisecond)
			if err != nil {
				logger.Printf("sync", "Error from actionstream: %v", err)
				continue
			}
			if !ok {
				continue
			}
			received++
			lastSeq = action.GlobalSeq
		}
	}
}
