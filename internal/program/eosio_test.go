package program

import (
	"bytes"
	"encoding/hex"
	"reflect"
	"testing"
	"time"

	"github.com/greymass/creationindex/internal/store"
	"github.com/greymass/roborovski/libraries/actionstream"
	"github.com/greymass/roborovski/libraries/chain"
)

func applyEosioFixture(t *testing.T, s *store.Store, actions []actionstream.Action, programAccount uint64) {
	t.Helper()
	b := s.NewBatch()
	for start := 0; start < len(actions); {
		end := start + 1
		for end < len(actions) && actions[end].TrxID == actions[start].TrxID {
			end++
		}
		if err := ApplyEosio(b, actions[start:end], programAccount); err != nil {
			t.Fatalf("trx %s: %v", actions[start].TrxID, err)
		}
		start = end
	}
	if err := s.Commit(b); err != nil {
		t.Fatal(err)
	}
}

func scanPrefix(t *testing.T, s *store.Store, prefix []byte, fn func(k, v []byte)) {
	t.Helper()
	iter, err := s.DB().NewIter(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer iter.Close()
	end := store.PrefixEnd(prefix)
	for iter.SeekGE(prefix); iter.Valid() && bytes.Compare(iter.Key(), end) < 0; iter.Next() {
		fn(iter.Key(), iter.Value())
	}
}

func programEvents(t *testing.T, s *store.Store) []store.ProgramEvent {
	t.Helper()
	var events []store.ProgramEvent
	scanPrefix(t, s, store.ProgramEventPrefix(), func(k, v []byte) {
		e, err := store.DecodeProgramEvent(v)
		if err != nil {
			t.Fatal(err)
		}
		if seq, ok := store.DecodeProgramEventKey(k); !ok || seq != e.GlobalSeq {
			t.Fatalf("event key %x holds seq %d", k, e.GlobalSeq)
		}
		events = append(events, *e)
	})
	return events
}

func balances(t *testing.T, s *store.Store) map[string]int64 {
	t.Helper()
	out := make(map[string]int64)
	scanPrefix(t, s, []byte{store.GiftedBalanceKey(0)[0]}, func(k, v []byte) {
		giftee, ok := store.DecodeGiftedBalanceKey(k)
		if !ok {
			t.Fatalf("bad balance key %x", k)
		}
		n, err := store.DecodeGiftedBalance(v)
		if err != nil {
			t.Fatal(err)
		}
		out[chain.NameToString(giftee)] = n
	})
	return out
}

func i64(v int64) *int64 { return &v }

func trx(t *testing.T, s string) [32]byte {
	t.Helper()
	var id [32]byte
	if _, err := hex.Decode(id[:], []byte(s)); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestEosioProgramEventsGiftGm(t *testing.T) {
	s := openStore(t)
	applyEosioFixture(t, s, loadFixture(t, "gift-gm-eosio"), name("gift.gm"))
	events := programEvents(t, s)
	if len(events) != 16 {
		t.Fatalf("%d events, want 16", len(events))
	}

	counts := make(map[store.EventType]int)
	var net int64
	for _, e := range events {
		counts[e.Type]++
		switch {
		case e.Type == store.EventProgramGift:
			net -= *e.Bytes
		case e.Type == store.EventReturn, e.To == name("gift.gm"):
			net += *e.Bytes
		case e.From == name("gift.gm"):
			net -= *e.Bytes
		}
	}
	if counts[store.EventProgramGift] != 13 || counts[store.EventReturn] != 1 || counts[store.EventRAMMovement] != 2 {
		t.Errorf("event counts %v", counts)
	}
	if net != 1221999 {
		t.Errorf("program RAM nets to %d, want the 1221999 logramchange reports after the last gift", net)
	}

	quantity, _ := chain.ParseAsset("102.1851 EOS")
	fee, _ := chain.ParseAsset("0.5110 EOS")
	want := map[uint64]store.ProgramEvent{
		365280805: {
			Type: store.EventRAMMovement, Action: name("buyrambytes"), GlobalSeq: 365280805, BlockNum: 279623247, BlockTime: 1678195830,
			TrxID: trx(t, "8b2ebda82c519d610455e6fc6d23f0f2b7ca79eee409ab1f3df4fd62d7cbab03"),
			Payer: name("gm"), To: name("gift.gm"), Bytes: i64(499999), Quantity: &quantity, Fee: &fee, InCreation: true,
		},
		365287401: {
			Type: store.EventRAMMovement, Action: name("ramtransfer"), GlobalSeq: 365287401, BlockNum: 279625088, BlockTime: 1678197671,
			TrxID: trx(t, "2e4c9ea1473160868db1fffef9ec394cf919228862e4777ba16d2b51cd7e8cb1"),
			From:  name("gm"), To: name("gift.gm"), Bytes: i64(1000000),
		},
		365335737: {
			Type: store.EventProgramGift, Action: name("giftram"), GlobalSeq: 365335737, BlockNum: 279640974, BlockTime: 1678213557,
			TrxID: trx(t, "78a288bef4bc97881a07baba5a5d5e150ba1be3e0fe973fcb87dae61492a7d68"),
			From:  name("gift.gm"), To: name("demouqteqjbz"), Bytes: i64(4000), Memo: "gifted account demo", InCreation: true,
		},
		396535542: {
			Type: store.EventReturn, Action: name("ungiftram"), GlobalSeq: 396535542, BlockNum: 292925796, BlockTime: 1691502952,
			TrxID: trx(t, "1ddef957020db81cbf97254e7d28af1832a5367ee3a7c53ff331524fc095e631"),
			From:  name("grbweziu.gm"), To: name("gift.gm"), Bytes: i64(50000), Memo: "return faucet gift before isolated source test",
		},
		396724550: {
			Type: store.EventProgramGift, Action: name("giftram"), GlobalSeq: 396724550, BlockNum: 293072493, BlockTime: 1691649785,
			TrxID: trx(t, "071ea115a6f4ceb339cb873cd5710b5e874ee8c83050f82c59bf96910c06d7e5"),
			From:  name("gift.gm"), To: name("reinaldo.gm"), Bytes: i64(4000), InCreation: true,
		},
	}
	for _, e := range events {
		w, ok := want[e.GlobalSeq]
		if !ok {
			continue
		}
		if !reflect.DeepEqual(e, w) {
			t.Errorf("seq %d:\n got %+v\nwant %+v", e.GlobalSeq, e, w)
		}
		delete(want, e.GlobalSeq)
	}
	if len(want) != 0 {
		t.Errorf("missing events %v", want)
	}
	if got := store.DayFromBlockTime(events[15].BlockTime); got != dayOf(t, "2026-10-20") {
		t.Errorf("reinaldo.gm gift on day %d", got)
	}

	wantBalances := map[string]int64{
		"demouqteqjbz": 4000, "demomikxduqu": 4000, "ssyfp5xf.gm": 4000, "btvxpqb4.gm": 4000,
		"hbxb1ygsw.gm": 50000, "memogame1.gm": 50000, "memogame2.gm": 50000, "grbwezi.gm": 50000,
		"uhy5ksrt5.gm": 50000, "foijhpiz3.gm": 4000, "eos3cold1.gm": 4000, "reinaldo.gm": 4000,
	}
	if got := balances(t, s); !reflect.DeepEqual(got, wantBalances) {
		t.Errorf("balances %v, want %v", got, wantBalances)
	}

	perDay := make(map[uint32][]uint64)
	scanPrefix(t, s, []byte{store.ProgramGiftDayPrefix(0)[0]}, func(k, v []byte) {
		day, seq, ok := store.DecodeProgramGiftDayKey(k)
		if !ok || len(v) != 0 {
			t.Fatalf("bad gift day entry %x = %x", k, v)
		}
		perDay[day] = append(perDay[day], seq)
	})
	wantDays := map[string]int{"2026-08-03": 2, "2026-10-18": 3, "2026-10-19": 6, "2026-10-20": 2}
	if len(perDay) != len(wantDays) {
		t.Errorf("gift days %v", perDay)
	}
	for day, n := range wantDays {
		if got := len(perDay[dayOf(t, day)]); got != n {
			t.Errorf("%s: %d gifts indexed, want %d", day, got, n)
		}
	}
	for _, seqs := range perDay {
		for _, seq := range seqs {
			if e := get(t, s, store.ProgramEventKey(seq)); e == nil {
				t.Errorf("gift day entry %d has no program event", seq)
			}
		}
	}
}

func TestEosioTopUpAndReturn(t *testing.T) {
	s := openStore(t)
	applyEosioFixture(t, s, loadFixture(t, "grbwezi-lease"), name("grbwezi.gm"))

	type row struct {
		seq        uint64
		typ        store.EventType
		from, to   string
		bytes      int64
		inCreation bool
	}
	want := []row{
		{396546843, store.EventRAMMovement, "grbwezi.gm", "grbuser11111", 8192, true},
		{396546850, store.EventProgramGift, "grbwezi.gm", "grbuser11111", 65536, true},
		{396546992, store.EventProgramGift, "grbwezi.gm", "grbuser11111", 16384, false},
		{396547019, store.EventRAMMovement, "grbwezi.gm", "grbuser22222", 8192, true},
		{396547026, store.EventProgramGift, "grbwezi.gm", "grbuser22222", 8192, true},
		{396547098, store.EventProgramGift, "grbwezi.gm", "grbuser11111", 1024, false},
		{396547132, store.EventReturn, "grbuser22222", "grbwezi.gm", 8192, false},
		{396547499, store.EventReturn, "grbuser11111", "grbwezi.gm", 65536 + 16384 + 1024, false},
	}
	events := programEvents(t, s)
	var got []row
	for _, e := range events {
		got = append(got, row{e.GlobalSeq, e.Type, chain.NameToString(e.From), chain.NameToString(e.To), *e.Bytes, e.InCreation})
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("events\n got %v\nwant %v", got, want)
	}
	if b := balances(t, s); len(b) != 0 {
		t.Errorf("balances %v left after both returns", b)
	}
}

func TestEosioWithoutProgramWritesNothing(t *testing.T) {
	for _, program := range []string{"", "gift.vaulta"} {
		s := openStore(t)
		applyEosioFixture(t, s, loadFixture(t, "gift-gm-eosio"), name(program))
		iter, err := s.DB().NewIter(nil)
		if err != nil {
			t.Fatal(err)
		}
		for iter.SeekGE([]byte{0x01}); iter.Valid(); iter.Next() {
			t.Errorf("program %q wrote key %x", program, iter.Key())
		}
		iter.Close()
	}
}

func dayOf(t *testing.T, day string) uint32 {
	t.Helper()
	d, err := time.Parse(time.DateOnly, day)
	if err != nil {
		t.Fatal(err)
	}
	return uint32(d.Unix() / 86400)
}
