package program

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/cockroachdb/pebble"
	"github.com/greymass/creationindex/internal/store"
	"github.com/greymass/roborovski/libraries/actionstream"
	"github.com/greymass/roborovski/libraries/chain"
)

func loadFixture(t *testing.T, name string) []actionstream.Action {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", name+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []actionstream.Action
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		var a actionstream.Action
		if err := json.Unmarshal(sc.Bytes(), &a); err != nil {
			t.Fatal(err)
		}
		out = append(out, a)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func applyFixture(t *testing.T, s *store.Store, actions []actionstream.Action) map[uint64]Outcome {
	t.Helper()
	g := NewGiftStream("gift.gm")
	b := s.NewBatch()
	outcomes := make(map[uint64]Outcome, len(actions))
	for i := range actions {
		o, err := g.Apply(b, &actions[i])
		if err != nil {
			t.Fatalf("seq %d: %v", actions[i].GlobalSeq, err)
		}
		outcomes[actions[i].GlobalSeq] = o
	}
	if err := s.Commit(b); err != nil {
		t.Fatal(err)
	}
	return outcomes
}

func openStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func get(t *testing.T, s *store.Store, k []byte) []byte {
	t.Helper()
	v, closer, err := s.DB().Get(k)
	if errors.Is(err, pebble.ErrNotFound) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	defer closer.Close()
	return append([]byte(nil), v...)
}

func name(s string) uint64 { return chain.StringToName(s) }

func TestGiftStreamOutcomes(t *testing.T) {
	s := openStore(t)
	outcomes := applyFixture(t, s, loadFixture(t, "gift-gm"))
	cases := []struct {
		seq  uint64
		want Outcome
	}{
		{365291613, Ignored},
		{365335736, Legacy},
		{365374411, Legacy},
		{396326781, Written},
		{396326782, Written},
		{396333028, Written},
		{396338596, Written},
		{396546322, Written},
		{396546776, Written},
		{396724549, Written},
	}
	for _, c := range cases {
		if got, ok := outcomes[c.seq]; !ok || got != c.want {
			t.Errorf("seq %d: outcome %v, want %v", c.seq, got, c.want)
		}
	}
}

func TestGiftStreamGiftAcctRows(t *testing.T) {
	s := openStore(t)
	applyFixture(t, s, loadFixture(t, "gift-gm"))

	v := get(t, s, store.GiftAcctKey(name("reinaldo.gm")))
	if v == nil {
		t.Fatal("no giftacct row for reinaldo.gm")
	}
	g, err := store.DecodeGiftAcct(v)
	if err != nil {
		t.Fatal(err)
	}
	want := store.GiftAcct{ProgramCreator: name("gm"), GlobalSeq: 396724549, BlockTime: g.BlockTime, Bytes: 4000}
	if *g != want || chain.Uint32ToTime(g.BlockTime) != "2026-10-20T15:21:32.500" {
		t.Errorf("reinaldo.gm row %+v at %s", g, chain.Uint32ToTime(g.BlockTime))
	}

	v = get(t, s, store.GiftAcctKey(name("uhy5ksrt5.gm")))
	if g, err = store.DecodeGiftAcct(v); err != nil || g.Bytes != 50000 || g.GlobalSeq != 396544427 {
		t.Errorf("uhy5ksrt5.gm row %+v, %v", g, err)
	}

	for _, legacy := range []string{"demouqteqjbz", "demomikxduqu"} {
		if get(t, s, store.GiftAcctKey(name(legacy))) != nil {
			t.Errorf("legacy giftacct to %s was written", legacy)
		}
	}
	if n := countPrefix(t, s, store.ProgramCreatorIndexPrefix(name("testuser.gm"))); n != 0 {
		t.Errorf("legacy operator testuser.gm has %d index entries", n)
	}
	if n := countPrefix(t, s, []byte{0x08}); n != 11 {
		t.Errorf("%d giftacct rows, want 11", n)
	}
}

func TestGiftStreamAdminEvents(t *testing.T) {
	s := openStore(t)
	applyFixture(t, s, loadFixture(t, "gift-gm"))

	q := func(v int64) *int64 { return &v }
	cases := []struct {
		seq     uint64
		action  string
		creator string
		quota   *int64
		trx     string
	}{
		{396326781, "enable", "", nil, "f6b596ca64579a7a"},
		{396326782, "addcreator", "gm", q(10000), "2ab2fb971b85c2d8"},
		{396338596, "setquota", "gm", q(1000000), "76ca5255718241e1"},
		{396544118, "rmcreator", "gm", nil, "ce92c157a46776fc"},
		{396544221, "addcreator", "gm", q(262144), "cadb01c1fced4cd5"},
		{396546322, "suspend", "gm", nil, "6e244c9eec5a8526"},
		{396546776, "resume", "gm", nil, "f325b932fc3ef320"},
	}
	for _, c := range cases {
		v := get(t, s, store.ProgramEventKey(c.seq))
		if v == nil {
			t.Errorf("seq %d: no program event", c.seq)
			continue
		}
		e, err := store.DecodeProgramEvent(v)
		if err != nil {
			t.Fatal(err)
		}
		if e.Type != store.EventAdmin || e.Action != name(c.action) || e.Creator != name(c.creator) || e.GlobalSeq != c.seq {
			t.Errorf("seq %d: event %+v", c.seq, e)
		}
		if (e.DailyQuotaBytes == nil) != (c.quota == nil) || (c.quota != nil && *e.DailyQuotaBytes != *c.quota) {
			t.Errorf("seq %d: daily_quota_bytes %v, want %v", c.seq, e.DailyQuotaBytes, c.quota)
		}
		if hex.EncodeToString(e.TrxID[:8]) != c.trx {
			t.Errorf("seq %d: trx_id %x, want %s...", c.seq, e.TrxID, c.trx)
		}
	}
	if n := countPrefix(t, s, store.ProgramEventPrefix()); n != len(cases) {
		t.Errorf("%d program events, want %d", n, len(cases))
	}
}

func countPrefix(t *testing.T, s *store.Store, prefix []byte) int {
	t.Helper()
	it, err := s.DB().NewIter(&pebble.IterOptions{LowerBound: prefix, UpperBound: store.PrefixEnd(prefix)})
	if err != nil {
		t.Fatal(err)
	}
	defer it.Close()
	n := 0
	for it.First(); it.Valid(); it.Next() {
		n++
	}
	return n
}

type window struct {
	start, end uint32
	gifts      int
	used       int64
}

func TestGiftStreamProgramCreatorIndex(t *testing.T) {
	s := openStore(t)
	applyFixture(t, s, loadFixture(t, "gift-gm"))

	prefix := store.ProgramCreatorIndexPrefix(name("gm"))
	it, err := s.DB().NewIter(&pebble.IterOptions{LowerBound: prefix, UpperBound: store.PrefixEnd(prefix)})
	if err != nil {
		t.Fatal(err)
	}
	defer it.Close()

	var seqs []uint64
	var windows [][]window
	for it.First(); it.Valid(); it.Next() {
		pc, seq, ok := store.DecodeProgramCreatorIndexKey(it.Key())
		if !ok || pc != name("gm") {
			t.Fatalf("bad index key %x", it.Key())
		}
		if len(seqs) > 0 && seq <= seqs[len(seqs)-1] {
			t.Fatalf("index out of order at %d", seq)
		}
		seqs = append(seqs, seq)
		entry, err := store.DecodeProgramCreatorEntry(it.Value())
		if err != nil {
			t.Fatal(err)
		}
		switch entry.Type {
		case store.EntryAdmin:
			e, err := store.DecodeProgramEvent(get(t, s, store.ProgramEventKey(seq)))
			if err != nil {
				t.Fatalf("admin entry %d: %v", seq, err)
			}
			if e.Action == name("addcreator") {
				windows = append(windows, nil)
			}
		case store.EntryGiftAcct:
			g, err := store.DecodeGiftAcct(get(t, s, store.GiftAcctKey(entry.Account)))
			if err != nil || g.GlobalSeq != seq {
				t.Fatalf("giftacct entry %d for %s: %+v, %v", seq, chain.NameToString(entry.Account), g, err)
			}
			reg := &windows[len(windows)-1]
			sec := g.BlockTime / 2
			if n := len(*reg); n == 0 || sec-(*reg)[n-1].start >= 86400 {
				*reg = append(*reg, window{start: sec, end: sec + 86400})
			}
			w := &(*reg)[len(*reg)-1]
			w.gifts++
			w.used += g.Bytes + 136
		}
	}
	if len(seqs) != 17 {
		t.Errorf("%d index entries for gm, want 17", len(seqs))
	}

	// Figures from the API routes prototype, checked there against the live creators row.
	want := [][]struct {
		gifts int
		used  int64
	}{
		{{5, 158680}, {2, 100272}},
		{{4, 62544}},
	}
	if len(windows) != len(want) {
		t.Fatalf("%d registrations, want %d", len(windows), len(want))
	}
	for r := range want {
		if len(windows[r]) != len(want[r]) {
			t.Fatalf("registration %d: %d windows, want %d", r, len(windows[r]), len(want[r]))
		}
		for i, w := range want[r] {
			if windows[r][i].gifts != w.gifts || windows[r][i].used != w.used {
				t.Errorf("registration %d window %d: %+v, want %+v", r, i, windows[r][i], w)
			}
		}
	}
}

func TestGiftStreamIgnoresOtherAccounts(t *testing.T) {
	s := openStore(t)
	g := NewGiftStream("gift.vaulta")
	b := s.NewBatch()
	defer b.Close()
	for _, a := range loadFixture(t, "gift-gm") {
		if o, err := g.Apply(b, &a); err != nil || o != Ignored {
			t.Fatalf("seq %d: %v, %v", a.GlobalSeq, o, err)
		}
	}
	if b.Count() != 0 {
		t.Errorf("batch holds %d writes", b.Count())
	}
}

func TestGiftStreamUndecodable(t *testing.T) {
	s := openStore(t)
	g := NewGiftStream("gift.gm")
	b := s.NewBatch()
	defer b.Close()
	for _, a := range loadFixture(t, "gift-gm") {
		if a.GlobalSeq != 396724549 && a.GlobalSeq != 396338596 {
			continue
		}
		a.ActionData = []byte("0000a0c8c9a46a4c")
		if _, err := g.Apply(b, &a); !errors.Is(err, ErrUndecodable) {
			t.Errorf("seq %d: err %v, want ErrUndecodable", a.GlobalSeq, err)
		}
	}
	if b.Count() != 0 {
		t.Errorf("batch holds %d writes", b.Count())
	}
}

func TestGiftFilter(t *testing.T) {
	f := GiftFilter(name("gift.vaulta"))
	if len(f.Contracts) != 1 || f.Contracts[0] != name("gift.vaulta") || len(f.Receivers) != 0 {
		t.Errorf("filter %+v", f)
	}
	want := map[uint64]bool{}
	for _, a := range []string{"giftacct", "addcreator", "setquota", "rmcreator", "suspend", "resume", "enable", "disable"} {
		want[name(a)] = true
	}
	if len(f.Actions) != len(want) {
		t.Fatalf("%d actions, want %d", len(f.Actions), len(want))
	}
	for _, a := range f.Actions {
		if !want[a] {
			t.Errorf("unexpected action %s", chain.NameToString(a))
		}
	}
}
