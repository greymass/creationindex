package main

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/greymass/creationindex/internal/store"
	"github.com/greymass/roborovski/libraries/actionstream"
	"github.com/greymass/roborovski/libraries/chain"
)

func loadFixture(t *testing.T, name string) []actionstream.Action {
	t.Helper()
	f, err := os.Open(filepath.Join("..", "..", "internal", "attribution", "testdata", name+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var actions []actionstream.Action
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		var a actionstream.Action
		if err := json.Unmarshal(sc.Bytes(), &a); err != nil {
			t.Fatal(err)
		}
		actions = append(actions, a)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return actions
}

func TestEosioTransactionsWrittenInOneBatch(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "creationindex.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var stream []actionstream.Action
	for _, name := range []string{"bios-no-contribution", "genesis-batch", "setacctram-outside-creation", "willsunshine"} {
		stream = append(stream, loadFixture(t, name)...)
	}

	var buf txBuffer
	var txs [][]actionstream.Action
	for _, a := range stream {
		if done := buf.push(a); done != nil {
			txs = append(txs, done)
		}
	}
	txs = append(txs, buf.take())
	if len(txs) != 4 {
		t.Fatalf("%d transactions, want 4", len(txs))
	}

	b := db.NewBatch()
	total := 0
	for _, tx := range txs {
		n, err := applyEosioTransaction(b, tx, chain.StringToName("gift.vaulta"))
		if err != nil {
			t.Fatal(err)
		}
		total += n
	}
	last := txs[3][len(txs[3])-1]
	if err := store.SetCursor(b, store.StreamEosio, store.Cursor{GlobalSeq: last.GlobalSeq, BlockTime: last.BlockTime}); err != nil {
		t.Fatal(err)
	}
	if got, _ := store.GetCreationCount(db.DB()); got != 0 {
		t.Fatalf("creation count %d visible before commit", got)
	}
	if err := db.Commit(b); err != nil {
		t.Fatal(err)
	}

	if total != 32 {
		t.Errorf("%d creations, want 32", total)
	}
	if got, err := store.GetCreationCount(db.DB()); err != nil || got != 32 {
		t.Errorf("creation count %d, %v; want 32", got, err)
	}
	c, err := store.GetCursor(db.DB(), store.StreamEosio)
	if err != nil || c.GlobalSeq != 368360304310 {
		t.Errorf("cursor %+v, %v; want seq 368360304310", c, err)
	}
	w, err := store.GetCreation(db.DB(), chain.StringToName("willsunshine"))
	if err != nil || w == nil || len(w.Contributions) != 1 || *w.Contributions[0].Bytes != 3299 {
		t.Fatalf("willsunshine %+v, %v", w, err)
	}
	m, err := store.GetCreation(db.DB(), chain.StringToName("eosio.msig"))
	if err != nil || m == nil || len(m.Contributions) != 0 {
		t.Fatalf("eosio.msig %+v, %v", m, err)
	}

	iter, err := db.DB().NewIter(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer iter.Close()
	prefix := store.CreatorIndexPrefix(chain.StringToName("eosio"))
	var indexed []uint64
	for iter.SeekGE(prefix); iter.Valid() && string(iter.Key()) < string(store.PrefixEnd(prefix)); iter.Next() {
		_, seq, ok := store.DecodeCreatorIndexKey(iter.Key())
		if !ok {
			t.Fatalf("bad creator index key %x", iter.Key())
		}
		indexed = append(indexed, seq)
	}
	if len(indexed) != 31 || indexed[0] != 134 || indexed[1] != 486 || indexed[30] != 921 {
		t.Errorf("eosio creator index seqs %v", indexed)
	}
}
