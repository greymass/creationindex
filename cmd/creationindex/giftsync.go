package main

import (
	"errors"
	"time"

	"github.com/cockroachdb/pebble"
	"github.com/greymass/creationindex/internal/program"
	"github.com/greymass/creationindex/internal/store"
	"github.com/greymass/roborovski/libraries/actionstream"
	"github.com/greymass/roborovski/libraries/chain"
	"github.com/greymass/roborovski/libraries/logger"
)

const giftBatchSize = 1000

type GiftSyncer struct {
	cfg    *Config
	store  *store.Store
	stream *program.GiftStream
	client *actionstream.Client
	stopCh chan struct{}
	done   chan struct{}
}

func NewGiftSyncer(cfg *Config, st *store.Store) *GiftSyncer {
	return &GiftSyncer{
		cfg:    cfg,
		store:  st,
		stream: program.NewGiftStream(cfg.ProgramAccount),
		stopCh: make(chan struct{}),
		done:   make(chan struct{}),
	}
}

func (s *GiftSyncer) Start() error {
	addr, err := streamAddress(s.cfg.ActionStreamSource)
	if err != nil {
		return err
	}
	cursor, err := store.GetCursor(s.store.DB(), store.StreamGift)
	if err != nil {
		return err
	}
	startSeq := cursor.GlobalSeq + 1

	clientCfg := actionstream.DefaultClientConfig()
	clientCfg.Debug = s.cfg.Debug
	clientCfg.Decode = true

	filter := program.GiftFilter(chain.StringToName(s.cfg.ProgramAccount))
	s.client = actionstream.NewClient(addr, filter, startSeq, clientCfg)
	if err := s.client.Connect(); err != nil {
		return err
	}
	logger.Printf("sync", "Gift stream on %s connected to %s, resuming from seq %d", s.cfg.ProgramAccount, addr, startSeq)

	go s.loop(cursor)
	return nil
}

func (s *GiftSyncer) Stop() {
	close(s.stopCh)
	if s.client != nil {
		s.client.Close()
	}
	<-s.done
}

func (s *GiftSyncer) loop(cursor store.Cursor) {
	defer close(s.done)

	var batch *pebble.Batch
	var pending int
	var received, written, legacy uint64

	flush := func() {
		if batch == nil {
			return
		}
		err := store.SetCursor(batch, store.StreamGift, cursor)
		if err == nil {
			err = s.store.Commit(batch)
		} else {
			batch.Close()
		}
		if err != nil {
			logger.Fatal("Gift stream commit at seq %d failed: %v", cursor.GlobalSeq, err)
		}
		logger.Printf("sync", "Gift stream committed %d actions, cursor=%d", pending, cursor.GlobalSeq)
		batch = nil
		pending = 0
	}
	defer flush()

	flushTicker := time.NewTicker(time.Second)
	defer flushTicker.Stop()
	statusTicker := time.NewTicker(10 * time.Second)
	defer statusTicker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-flushTicker.C:
			flush()
		case <-statusTicker.C:
			logger.Printf("status", "Gift stream: received=%d written=%d legacy=%d cursor=%d head=%d catchup=%v",
				received, written, legacy, cursor.GlobalSeq, s.client.GetHeadSeq(), s.client.IsCatchupComplete())
		default:
			action, ok, err := s.client.NextWithTimeout(100 * time.Millisecond)
			if err != nil {
				if errors.Is(err, actionstream.ErrAlreadyClosed) {
					return
				}
				logger.Printf("sync", "Gift stream error from actionstream: %v", err)
				continue
			}
			if !ok {
				continue
			}
			received++
			if batch == nil {
				batch = s.store.NewBatch()
			}
			outcome, err := s.stream.Apply(batch, &action)
			switch {
			case errors.Is(err, program.ErrUndecodable):
				logger.Printf("sync", "Gift stream skipping %s::%s: %v", action.Contract, action.Action, err)
			case err != nil:
				logger.Fatal("Gift stream write at seq %d failed: %v", action.GlobalSeq, err)
			case outcome == program.Written:
				written++
			case outcome == program.Legacy:
				legacy++
				logger.Printf("sync", "Gift stream skipping legacy %s::%s seq=%d", action.Contract, action.Action, action.GlobalSeq)
			}
			cursor = store.Cursor{GlobalSeq: action.GlobalSeq, BlockTime: action.BlockTime}
			pending++
			if pending >= giftBatchSize {
				flush()
			}
		}
	}
}
