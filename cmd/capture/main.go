package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/greymass/creationindex/internal/attribution"
	"github.com/greymass/roborovski/libraries/actionstream"
)

var streams = map[string]func() actionstream.Filter{
	"eosio": attribution.Filter,
}

func main() {
	source := flag.String("source", "stream://localhost:9081", "actionstream source (stream://host:port or stream:///path.sock)")
	stream := flag.String("stream", "eosio", "service stream whose filter to subscribe with")
	start := flag.Uint64("start", 1, "global seq to subscribe from")
	out := flag.String("out", "testdata", "directory for <name>.jsonl files")
	idle := flag.Duration("idle", 10*time.Second, "stop after this long without an action once every named transaction is seen")
	timeout := flag.Duration("timeout", 30*time.Minute, "give up after this long")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: capture [flags] name=trx_id ...\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if err := run(*source, *stream, *start, *out, *idle, *timeout, flag.Args()); err != nil {
		fmt.Fprintf(os.Stderr, "capture: %v\n", err)
		os.Exit(1)
	}
}

func run(source, stream string, start uint64, out string, idle, timeout time.Duration, args []string) error {
	filter, ok := streams[stream]
	if !ok {
		return fmt.Errorf("unknown stream %q", stream)
	}
	if len(args) == 0 {
		return fmt.Errorf("name at least one name=trx_id")
	}
	names := make(map[string]string, len(args))
	for _, arg := range args {
		name, trx, ok := strings.Cut(arg, "=")
		if !ok || name == "" || len(trx) != 64 {
			return fmt.Errorf("invalid argument %q: want name=trx_id", arg)
		}
		names[strings.ToLower(trx)] = name
	}
	addr, ok := strings.CutPrefix(source, "stream://")
	if !ok || addr == "" {
		return fmt.Errorf("invalid source %q", source)
	}

	cfg := actionstream.DefaultClientConfig()
	cfg.Decode = true
	client := actionstream.NewClient(addr, filter(), start, cfg)
	if err := client.Connect(); err != nil {
		return err
	}
	defer client.Close()

	captured := make(map[string][]actionstream.Action)
	deadline := time.Now().Add(timeout)
	lastAction := time.Now()
	var received uint64
	var current string
	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out after %d actions with %d of %d transactions seen", received, len(captured), len(names))
		}
		action, got, err := client.NextWithTimeout(time.Second)
		if err != nil {
			return err
		}
		if !got {
			if len(captured) == len(names) && time.Since(lastAction) >= idle {
				break
			}
			continue
		}
		received++
		lastAction = time.Now()
		if action.TrxID != current && len(captured) == len(names) {
			break
		}
		current = action.TrxID
		if _, ok := names[action.TrxID]; ok {
			captured[action.TrxID] = append(captured[action.TrxID], action)
		}
		if received%1000000 == 0 {
			fmt.Fprintf(os.Stderr, "received %d actions, seq %d, %d of %d transactions seen\n", received, action.GlobalSeq, len(captured), len(names))
		}
	}

	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	trxs := make([]string, 0, len(captured))
	for trx := range captured {
		trxs = append(trxs, trx)
	}
	sort.Strings(trxs)
	for _, trx := range trxs {
		path := filepath.Join(out, names[trx]+".jsonl")
		if err := write(path, captured[trx]); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "wrote %s: %d actions, seq %d to %d\n", path, len(captured[trx]), captured[trx][0].GlobalSeq, captured[trx][len(captured[trx])-1].GlobalSeq)
	}
	return nil
}

func write(path string, actions []actionstream.Action) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	enc := json.NewEncoder(w)
	for i := range actions {
		if err := enc.Encode(&actions[i]); err != nil {
			f.Close()
			return err
		}
	}
	if err := w.Flush(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
