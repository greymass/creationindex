package attribution

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"

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

type record struct {
	Account       string
	Creator       string
	BlockNum      uint32
	BlockTime     uint32
	TrxID         string
	GlobalSeq     uint64
	Contributions []contrib
}

type contrib struct {
	Kind           string
	Action         string
	GlobalSeq      uint64
	Payer          string
	Bytes          any
	RequestedBytes any
	Quantity       any
	Fee            any
	Memo           string
}

func optInt(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

func optAsset(v *chain.Asset) any {
	if v == nil {
		return nil
	}
	return v.String()
}

func render(c store.Creation) record {
	r := record{
		Account:   chain.NameToString(c.Account),
		Creator:   chain.NameToString(c.Creator),
		BlockNum:  c.BlockNum,
		BlockTime: c.BlockTime,
		TrxID:     hex.EncodeToString(c.TrxID[:]),
		GlobalSeq: c.GlobalSeq,
	}
	for _, k := range c.Contributions {
		r.Contributions = append(r.Contributions, contrib{
			Kind:           k.Kind.String(),
			Action:         chain.NameToString(k.Action),
			GlobalSeq:      k.GlobalSeq,
			Payer:          chain.NameToString(k.Payer),
			Bytes:          optInt(k.Bytes),
			RequestedBytes: optInt(k.RequestedBytes),
			Quantity:       optAsset(k.Quantity),
			Fee:            optAsset(k.Fee),
			Memo:           k.Memo,
		})
	}
	return r
}

func isoTime(blockTime uint32) string {
	return time.Unix(946684800+int64(blockTime/2), int64(blockTime%2)*int64(500*time.Millisecond)).UTC().Format("2006-01-02T15:04:05.000Z")
}

func i64(v int64) any { return v }

func TestFixtures(t *testing.T) {
	cases := []struct {
		fixture string
		program string
		want    []record
		time    string
	}{
		{
			fixture: "bios-no-contribution",
			program: "gift.vaulta",
			time:    "2018-06-09T11:57:35.000Z",
			want: []record{{
				Account: "eosio.msig", Creator: "eosio", BlockNum: 132, BlockTime: 1163721310, GlobalSeq: 134,
				TrxID: "708f08725f1e7f86670a3e4f5e01f8833df875b5949513643a0a362e3c6ac75f",
			}},
		},
		{
			fixture: "buyram-pre-v330",
			program: "gift.vaulta",
			time:    "2018-07-10T13:56:41.500Z",
			want: []record{{
				Account: "cibcibcibcib", Creator: "signupeoseos", BlockNum: 5160201, BlockTime: 1169092403, GlobalSeq: 16092431,
				TrxID:         "a794a6262b98ece5dae2edd33b3d80b09d9ec902e517161f7d5ce6bdba7adf24",
				Contributions: []contrib{{Kind: "buy", Action: "buyram", GlobalSeq: 16092432, Payer: "signupeoseos", Quantity: "1.7900 EOS"}},
			}},
		},
		{
			fixture: "buyrambytes-pre-v330",
			program: "gift.vaulta",
			time:    "2023-08-12T11:49:00.000Z",
			want: []record{{
				Account: "rentikm12345", Creator: "genialwombat", BlockNum: 325305684, BlockTime: 1490312280, GlobalSeq: 358841473278,
				TrxID:         "00340dc3703706b9285a108a7e9e442081b1f2d3152fa2d7034ffb30be806712",
				Contributions: []contrib{{Kind: "buy", Action: "buyrambytes", GlobalSeq: 358841473279, Payer: "genialwombat", RequestedBytes: i64(1739)}},
			}},
		},
		{
			fixture: "buyrambytes-v330-no-fee",
			program: "gift.vaulta",
			time:    "2024-04-22T19:08:42.000Z",
			want: []record{{
				Account: "zorbenzorben", Creator: "genialwombat", BlockNum: 369214245, BlockTime: 1534256244, GlobalSeq: 360594892990,
				TrxID:         "01b4de36c00a6b10cd914dc0b806c4a4c9822992dd5e0f32e7b60796f1aa0ba3",
				Contributions: []contrib{{Kind: "buy", Action: "buyrambytes", GlobalSeq: 360594892991, Payer: "genialwombat", Bytes: i64(1738), Quantity: "0.5493 EOS"}},
			}},
		},
		{
			fixture: "willsunshine",
			program: "gift.vaulta",
			time:    "2026-10-20T22:29:07.000Z",
			want: []record{{
				Account: "willsunshine", Creator: "openvaccount", BlockNum: 526448146, BlockTime: 1691701094, GlobalSeq: 368360304290,
				TrxID:         "3efce2e5432e2e1ce595615e19fa245118e6766414022bc935a7a8eebfa79258",
				Contributions: []contrib{{Kind: "buy", Action: "buyrambytes", GlobalSeq: 368360304296, Payer: "openvaccount", Bytes: i64(3299), Quantity: "1.1225 EOS", Fee: "0.0057 EOS"}},
			}},
		},
		{
			fixture: "aaaisn.b.sat",
			program: "gift.vaulta",
			time:    "2026-10-18T16:01:10.500Z",
			want: []record{{
				Account: "aaaisn.b.sat", Creator: "sat", BlockNum: 526056215, BlockTime: 1691308941, GlobalSeq: 368348901051,
				TrxID:         "51913702421ccde6dc815fa9e65fe16a3ae21345125438717a511fd00bfa160b",
				Contributions: []contrib{{Kind: "gift", Action: "giftram", GlobalSeq: 368348901052, Payer: "signup.xsat", Bytes: i64(6144), Memo: "Bank account signup"}},
			}},
		},
		{
			fixture: "ramtransfer-wrapped",
			program: "gift.vaulta",
			time:    "2025-05-14T16:19:58.500Z",
			want: []record{{
				Account: "swapstesting", Creator: "nsjames.gm", BlockNum: 435962638, BlockTime: 1601109597, GlobalSeq: 363776595330,
				TrxID:         "2e2ed1e29875d037b0d67acfa752e4a286e144d8e0f02b407d253a92938328ad",
				Contributions: []contrib{{Kind: "transfer", Action: "ramtransfer", GlobalSeq: 363776595332, Payer: "nsjames.gm", Bytes: i64(2996)}},
			}},
		},
		{
			fixture: "reinaldo.gm",
			program: "gift.gm",
			time:    "2026-10-20T15:21:32.500Z",
			want: []record{{
				Account: "reinaldo.gm", Creator: "gm", BlockNum: 293072493, BlockTime: 1691649785, GlobalSeq: 396724548,
				TrxID:         "071ea115a6f4ceb339cb873cd5710b5e874ee8c83050f82c59bf96910c06d7e5",
				Contributions: []contrib{{Kind: "program_gift", Action: "giftram", GlobalSeq: 396724550, Payer: "gift.gm", Bytes: i64(4000)}},
			}},
		},
		{
			fixture: "jungle4-bios-undecoded",
			program: "gift.gm",
			time:    "2022-02-22T22:22:22.500Z",
			want: []record{{
				Account: "eosio.bpay", Creator: "eosio", BlockNum: 2, BlockTime: 1397767485, GlobalSeq: 2,
				TrxID: "30268a7604a527c62b31d3d2bd4d04ef420ed2e3b98a297187379d25081addef",
			}},
		},
		{
			fixture: "setacctram-outside-creation",
			program: "gift.vaulta",
		},
	}
	for _, tc := range cases {
		t.Run(tc.fixture, func(t *testing.T) {
			got, err := Attribute(loadFixture(t, tc.fixture), chain.StringToName(tc.program))
			if err != nil {
				t.Fatal(err)
			}
			var rendered []record
			for _, c := range got {
				rendered = append(rendered, render(c))
			}
			if !reflect.DeepEqual(rendered, tc.want) {
				t.Fatalf("got  %+v\nwant %+v", rendered, tc.want)
			}
			if len(got) > 0 && isoTime(got[0].BlockTime) != tc.time {
				t.Errorf("block time %s, want %s", isoTime(got[0].BlockTime), tc.time)
			}
		})
	}
}

func TestGenesisBatch(t *testing.T) {
	got, err := Attribute(loadFixture(t, "genesis-batch"), chain.StringToName("gift.vaulta"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 30 {
		t.Fatalf("%d creations, want 30", len(got))
	}
	first := render(got[0])
	if first.Account != "heztcnjuhege" || first.GlobalSeq != 486 || first.BlockNum != 447 {
		t.Errorf("first creation %+v", first)
	}
	last := render(got[29])
	if last.Account != "heztcnbxgqge" || last.GlobalSeq != 921 {
		t.Errorf("last creation %+v", last)
	}
	seen := make(map[uint64]bool)
	for i, c := range got {
		if i > 0 && c.GlobalSeq <= got[i-1].GlobalSeq {
			t.Errorf("creation %d out of seq order", i)
		}
		if seen[c.Account] {
			t.Errorf("%s created twice", chain.NameToString(c.Account))
		}
		seen[c.Account] = true
		r := render(c)
		want := []contrib{{Kind: "buy", Action: "buyrambytes", GlobalSeq: c.GlobalSeq + 1, Payer: "eosio", RequestedBytes: i64(8192)}}
		if r.Creator != "eosio" || !reflect.DeepEqual(r.Contributions, want) {
			t.Errorf("creation %d: %+v", i, r)
		}
	}
}

func TestProgramAccountFromConfig(t *testing.T) {
	got, err := Attribute(loadFixture(t, "reinaldo.gm"), chain.StringToName("gift.vaulta"))
	if err != nil {
		t.Fatal(err)
	}
	if k := got[0].Contributions[0].Kind; k != store.KindGift {
		t.Errorf("gift.gm giftram on a gift.vaulta instance is %s, want gift", k)
	}
	got, err = Attribute(loadFixture(t, "aaaisn.b.sat"), chain.StringToName("signup.xsat"))
	if err != nil {
		t.Fatal(err)
	}
	if k := got[0].Contributions[0].Kind; k != store.KindProgramGift {
		t.Errorf("signup.xsat giftram with signup.xsat as program account is %s, want program_gift", k)
	}
}

func TestNotificationCopiesIgnored(t *testing.T) {
	actions := loadFixture(t, "willsunshine")
	for _, a := range actions[1:] {
		a.Receiver = "openvaccount"
		a.GlobalSeq += 1
		actions = append(actions, a)
	}
	sort.Slice(actions, func(i, j int) bool { return actions[i].GlobalSeq < actions[j].GlobalSeq })
	got, err := Attribute(actions, chain.StringToName("gift.vaulta"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || len(got[0].Contributions) != 1 || *got[0].Contributions[0].Bytes != 3299 {
		t.Fatalf("got %+v", got)
	}
}
