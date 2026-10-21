package main

import (
	"time"

	"github.com/cockroachdb/pebble"
	"github.com/greymass/creationindex/internal/attribution"
	"github.com/greymass/creationindex/internal/store"
	"github.com/greymass/roborovski/libraries/actionstream"
	"github.com/greymass/roborovski/libraries/chain"
	"github.com/greymass/roborovski/libraries/logger"
)

const (
	eosioIdleFlush    = time.Second
	eosioCommitEvery  = 1000
	eosioCommitPeriod = time.Second
)

type txBuffer struct {
	actions []actionstream.Action
}

// push returns the open transaction when a differs from it, then opens a's.
func (t *txBuffer) push(a actionstream.Action) []actionstream.Action {
	var done []actionstream.Action
	if len(t.actions) > 0 && t.actions[0].TrxID != a.TrxID {
		done = t.take()
	}
	t.actions = append(t.actions, a)
	return done
}

func (t *txBuffer) take() []actionstream.Action {
	done := t.actions
	t.actions = nil
	return done
}

func applyEosioTransaction(b *pebble.Batch, actions []actionstream.Action, program uint64) (int, error) {
	creations, err := attribution.Attribute(actions, program)
	if err != nil || len(creations) == 0 {
		return 0, err
	}
	count, err := store.GetCreationCount(b)
	if err != nil {
		return 0, err
	}
	for i := range creations {
		if err := store.PutCreation(b, &creations[i]); err != nil {
			return 0, err
		}
	}
	return len(creations), store.SetCreationCount(b, count+uint64(len(creations)))
}

type eosioSyncer struct {
	db      *store.Store
	cfg     *Config
	program uint64
	client  *actionstream.Client
	stopCh  chan struct{}
	done    chan struct{}
}

func newEosioSyncer(db *store.Store, cfg *Config) *eosioSyncer {
	return &eosioSyncer{
		db:      db,
		cfg:     cfg,
		program: chain.StringToName(cfg.ProgramAccount),
		stopCh:  make(chan struct{}),
		done:    make(chan struct{}),
	}
}

func (s *eosioSyncer) Start() error {
	addr, err := streamAddress(s.cfg.ActionStreamSource)
	if err != nil {
		return err
	}
	cursor, err := store.GetCursor(s.db.DB(), store.StreamEosio)
	if err != nil {
		return err
	}
	start := cursor.GlobalSeq + 1

	clientCfg := actionstream.DefaultClientConfig()
	clientCfg.Debug = s.cfg.Debug
	clientCfg.Decode = true

	s.client = actionstream.NewClient(addr, attribution.Filter(), start, clientCfg)
	if err := s.client.Connect(); err != nil {
		return err
	}
	logger.Printf("sync", "eosio stream connected to %s, resuming from seq %d", addr, start)

	go s.loop(cursor)
	return nil
}

func (s *eosioSyncer) Stop() {
	close(s.stopCh)
	if s.client != nil {
		s.client.Close()
	}
	<-s.done
}

func (s *eosioSyncer) loop(cursor store.Cursor) {
	defer close(s.done)

	var (
		buf                 txBuffer
		batch               *pebble.Batch
		pendingTxs          int
		received, creations uint64
		lastAction          time.Time
		lastTrx             string
	)

	commit := func() {
		if batch == nil {
			return
		}
		if err := store.SetCursor(batch, store.StreamEosio, cursor); err != nil {
			logger.Fatal("eosio stream: set cursor: %v", err)
		}
		if err := s.db.Commit(batch); err != nil {
			logger.Fatal("eosio stream: commit at seq %d: %v", cursor.GlobalSeq, err)
		}
		batch = nil
		pendingTxs = 0
	}

	process := func(actions []actionstream.Action) {
		if len(actions) == 0 {
			return
		}
		if batch == nil {
			batch = s.db.NewBatch()
		}
		n, err := applyEosioTransaction(batch, actions, s.program)
		if err != nil {
			logger.Fatal("eosio stream: transaction %s: %v", actions[0].TrxID, err)
		}
		last := actions[len(actions)-1]
		cursor = store.Cursor{GlobalSeq: last.GlobalSeq, BlockTime: last.BlockTime}
		creations += uint64(n)
		lastTrx = last.TrxID
		pendingTxs++
		if pendingTxs >= eosioCommitEvery {
			commit()
		}
	}

	commitTicker := time.NewTicker(eosioCommitPeriod)
	defer commitTicker.Stop()
	statusTicker := time.NewTicker(10 * time.Second)
	defer statusTicker.Stop()

	for {
		select {
		case <-s.stopCh:
			commit()
			return
		case <-commitTicker.C:
			commit()
		case <-statusTicker.C:
			logger.Printf("status", "eosio stream: actions_received=%d creations=%d cursor=%d head=%d lib=%d catchup=%v",
				received, creations, cursor.GlobalSeq, s.client.GetHeadSeq(), s.client.GetLIBSeq(), s.client.IsCatchupComplete())
		default:
			action, ok, err := s.client.NextWithTimeout(100 * time.Millisecond)
			if err != nil {
				logger.Printf("sync", "eosio stream: %v", err)
				continue
			}
			if !ok {
				if len(buf.actions) > 0 && time.Since(lastAction) >= eosioIdleFlush {
					process(buf.take())
				}
				continue
			}
			received++
			lastAction = time.Now()
			if action.TrxID == lastTrx {
				logger.Printf("sync", "eosio stream: transaction %s continued after an idle flush at seq %d", action.TrxID, action.GlobalSeq)
			}
			process(buf.push(action))
		}
	}
}
