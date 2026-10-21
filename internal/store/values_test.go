package store

import (
	"errors"
	"reflect"
	"testing"

	"github.com/greymass/roborovski/libraries/chain"
)

func i64(v int64) *int64 { return &v }

func eos(amount int64) *chain.Asset {
	return &chain.Asset{Amount: amount, Symbol: 4 | chain.StringToSymbolCode("EOS")<<8}
}

func trx(b byte) (t [32]byte) {
	for i := range t {
		t[i] = b + byte(i)
	}
	return t
}

func TestCreationRoundTrip(t *testing.T) {
	cases := []*Creation{
		{},
		{Account: chain.StringToName("eosio.token"), Creator: chain.StringToName("eosio"), BlockNum: 1, BlockTime: 2, TrxID: trx(0)},
		{
			Account: chain.StringToName("willsunshine"), Creator: chain.StringToName("openvaccount"),
			BlockNum: 526448146, BlockTime: 1689193748, TrxID: trx(0x3e), GlobalSeq: 368360304290,
			Contributions: []Contribution{
				{Kind: KindBuy, Action: chain.StringToName("buyrambytes"), GlobalSeq: 368360304296, Payer: chain.StringToName("openvaccount"), Bytes: i64(3299), Quantity: eos(11225), Fee: eos(57)},
				{Kind: KindBuy, Action: chain.StringToName("buyrambytes"), GlobalSeq: 7, Payer: 1, RequestedBytes: i64(8192)},
				{Kind: KindBuy, Action: chain.StringToName("buyram"), GlobalSeq: 8, Payer: 1, Quantity: eos(0)},
				{Kind: KindGift, Action: chain.StringToName("giftram"), GlobalSeq: 9, Payer: 2, Bytes: i64(6144), Memo: "Bank account signup"},
				{Kind: KindProgramGift, Action: chain.StringToName("giftram"), GlobalSeq: 10, Payer: 3, Bytes: i64(0)},
				{Kind: KindTransfer, Action: chain.StringToName("ramtransfer"), GlobalSeq: 11, Payer: 4, Bytes: i64(1 << 40), Memo: "ü"},
				{Kind: KindQuotaSet, Action: chain.StringToName("setacctram"), GlobalSeq: 12, Bytes: i64(-1)},
			},
		},
	}
	for i, c := range cases {
		got, err := DecodeCreation(EncodeCreation(c))
		if err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
		if !reflect.DeepEqual(got, c) {
			t.Errorf("case %d: round trip\n got %+v\nwant %+v", i, got, c)
		}
	}
}

func TestContributionNullsStayNull(t *testing.T) {
	c := &Creation{Contributions: []Contribution{{Kind: KindBuy}}}
	got, err := DecodeCreation(EncodeCreation(c))
	if err != nil {
		t.Fatal(err)
	}
	g := got.Contributions[0]
	if g.Bytes != nil || g.RequestedBytes != nil || g.Quantity != nil || g.Fee != nil || g.Memo != "" {
		t.Errorf("unset figures came back set: %+v", g)
	}
}

func TestDailyFiguresRoundTrip(t *testing.T) {
	full := &DailyFigures{Creations: 1840, CreationsWithoutRAM: 3}
	full.Kind(KindBuy).Contributions = 1802
	full.Kind(KindBuy).WithoutBytes = 1802
	full.Kind(KindBuy).RequestedBytes = 5420000
	full.Kind(KindBuy).Quantity = *eos(3104402)
	full.Kind(KindQuotaSet).Contributions = 38
	full.Kind(KindProgramGift).Bytes = 1 << 50
	for i, d := range []*DailyFigures{{}, full} {
		got, err := DecodeDailyFigures(EncodeDailyFigures(d))
		if err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
		if !reflect.DeepEqual(got, d) {
			t.Errorf("case %d: round trip\n got %+v\nwant %+v", i, got, d)
		}
	}
}

func TestProgramEventRoundTrip(t *testing.T) {
	gm, giftgm := chain.StringToName("gm"), chain.StringToName("gift.gm")
	cases := []*ProgramEvent{
		{Type: EventAdmin, Action: chain.StringToName("enable")},
		{Type: EventAdmin, Action: chain.StringToName("setquota"), GlobalSeq: 1, BlockNum: 2, BlockTime: 3, TrxID: trx(1), Creator: gm, DailyQuotaBytes: i64(1000000)},
		{Type: EventProgramGift, Action: chain.StringToName("giftram"), GlobalSeq: 396724550, From: giftgm, To: chain.StringToName("reinaldo.gm"), Bytes: i64(4000), InCreation: true},
		{Type: EventProgramGift, Action: chain.StringToName("giftram"), From: giftgm, To: 5, Bytes: i64(50000), Memo: "top-up"},
		{Type: EventReturn, Action: chain.StringToName("ungiftram"), From: chain.StringToName("grbweziu.gm"), To: giftgm, Bytes: i64(50000), Memo: "return faucet gift before isolated source test"},
		{Type: EventRAMMovement, Action: chain.StringToName("buyrambytes"), Payer: gm, To: giftgm, Bytes: i64(499999), Quantity: eos(1021851), Fee: eos(5110)},
	}
	for i, e := range cases {
		got, err := DecodeProgramEvent(EncodeProgramEvent(e))
		if err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
		if !reflect.DeepEqual(got, e) {
			t.Errorf("case %d: round trip\n got %+v\nwant %+v", i, got, e)
		}
	}
}

func TestSmallValuesRoundTrip(t *testing.T) {
	for _, g := range []*GiftAcct{{}, {ProgramCreator: nameGm, GlobalSeq: 396724549, BlockTime: 1, Bytes: 4000, Memo: "hello"}} {
		got, err := DecodeGiftAcct(EncodeGiftAcct(g))
		if err != nil || !reflect.DeepEqual(got, g) {
			t.Errorf("giftacct %+v = %+v, %v", g, got, err)
		}
	}
	for _, v := range []int64{0, 4000, 1 << 62} {
		got, err := DecodeGiftedBalance(EncodeGiftedBalance(v))
		if err != nil || got != v {
			t.Errorf("gifted balance %d = %d, %v", v, got, err)
		}
	}
	for _, c := range []Cursor{{}, {GlobalSeq: 368360752482, BlockTime: ^uint32(0)}} {
		got, err := DecodeCursor(EncodeCursor(c))
		if err != nil || got != c {
			t.Errorf("cursor %+v = %+v, %v", c, got, err)
		}
	}
	for _, e := range []ProgramCreatorEntry{{Type: EntryAdmin}, {Type: EntryGiftAcct, Account: nameMax}} {
		got, err := DecodeProgramCreatorEntry(EncodeProgramCreatorEntry(e))
		if err != nil || got != e {
			t.Errorf("program creator entry %+v = %+v, %v", e, got, err)
		}
	}
	for _, n := range nameSamples {
		got, err := DecodeName(EncodeName(n))
		if err != nil || got != n {
			t.Errorf("name %d = %d, %v", n, got, err)
		}
	}
}

func TestDecodeRejectsCorruptValues(t *testing.T) {
	creation := EncodeCreation(&Creation{Contributions: []Contribution{{Kind: KindGift, Bytes: i64(1), Memo: "m"}}})
	decoders := map[string]struct {
		valid  []byte
		decode func([]byte) error
	}{
		"creation":      {creation, func(b []byte) error { _, err := DecodeCreation(b); return err }},
		"daily":         {EncodeDailyFigures(&DailyFigures{}), func(b []byte) error { _, err := DecodeDailyFigures(b); return err }},
		"program event": {EncodeProgramEvent(&ProgramEvent{Type: EventReturn, Memo: "m"}), func(b []byte) error { _, err := DecodeProgramEvent(b); return err }},
		"giftacct":      {EncodeGiftAcct(&GiftAcct{Memo: "m"}), func(b []byte) error { _, err := DecodeGiftAcct(b); return err }},
		"balance":       {EncodeGiftedBalance(300), func(b []byte) error { _, err := DecodeGiftedBalance(b); return err }},
		"cursor":        {EncodeCursor(Cursor{}), func(b []byte) error { _, err := DecodeCursor(b); return err }},
		"entry":         {EncodeProgramCreatorEntry(ProgramCreatorEntry{Type: EntryAdmin}), func(b []byte) error { _, err := DecodeProgramCreatorEntry(b); return err }},
	}
	for name, d := range decoders {
		for n := 0; n < len(d.valid); n++ {
			if err := d.decode(d.valid[:n]); !errors.Is(err, ErrCorrupt) {
				t.Errorf("%s truncated to %d: err = %v", name, n, err)
			}
		}
		if err := d.decode(append(append([]byte(nil), d.valid...), 0)); !errors.Is(err, ErrCorrupt) {
			t.Errorf("%s with a trailing byte: err = %v", name, err)
		}
		bumped := append([]byte(nil), d.valid...)
		bumped[0]++
		if err := d.decode(bumped); !errors.Is(err, ErrCorrupt) {
			t.Errorf("%s with a future version: err = %v", name, err)
		}
	}
	bad := EncodeCreation(&Creation{Contributions: []Contribution{{Kind: 9}}})
	if _, err := DecodeCreation(bad); !errors.Is(err, ErrCorrupt) {
		t.Errorf("unknown kind: err = %v", err)
	}
	if _, err := DecodeProgramEvent(EncodeProgramEvent(&ProgramEvent{})); !errors.Is(err, ErrCorrupt) {
		t.Errorf("zero event type: err = %v", err)
	}
}

func TestKindAndEventNames(t *testing.T) {
	want := []string{"buy", "transfer", "gift", "program_gift", "quota_set"}
	for i, w := range want {
		if got := Kind(i + 1).String(); got != w {
			t.Errorf("Kind(%d) = %q, want %q", i+1, got, w)
		}
	}
	events := []string{"admin", "program_gift", "return", "ram_movement"}
	for i, w := range events {
		if got := EventType(i + 1).String(); got != w {
			t.Errorf("EventType(%d) = %q, want %q", i+1, got, w)
		}
	}
}
