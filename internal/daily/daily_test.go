package daily

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/cockroachdb/pebble"
	"github.com/greymass/creationindex/internal/attribution"
	"github.com/greymass/creationindex/internal/store"
	"github.com/greymass/roborovski/libraries/actionstream"
	"github.com/greymass/roborovski/libraries/chain"
)

func loadFixture(t *testing.T, name string) []actionstream.Action {
	t.Helper()
	f, err := os.Open(filepath.Join("..", "attribution", "testdata", name+".jsonl"))
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

type fig struct {
	Contributions, Bytes, WithoutBytes, RequestedBytes uint64
	Quantity                                           string
}

type row struct {
	Creations, CreationsWithoutRAM uint64
	Kinds                          map[string]fig
}

func render(d *store.DailyFigures) row {
	r := row{Creations: d.Creations, CreationsWithoutRAM: d.CreationsWithoutRAM, Kinds: map[string]fig{}}
	for k := store.KindBuy; k <= store.KindQuotaSet; k++ {
		f := d.Kind(k)
		if *f == (store.Figures{}) {
			continue
		}
		q := ""
		if f.Quantity != (chain.Asset{}) {
			q = f.Quantity.String()
		}
		r.Kinds[k.String()] = fig{f.Contributions, f.Bytes, f.WithoutBytes, f.RequestedBytes, q}
	}
	return r
}

func day(t *testing.T, s string) uint32 {
	t.Helper()
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatal(err)
	}
	return uint32(d.Unix() / 86400)
}

func apply(t *testing.T, b *pebble.Batch, fixture, program string) {
	t.Helper()
	creations, err := attribution.Attribute(loadFixture(t, fixture), chain.StringToName(program))
	if err != nil {
		t.Fatal(err)
	}
	if err := Apply(b, creations); err != nil {
		t.Fatal(err)
	}
}

func getRow(t *testing.T, r pebble.Reader, k []byte) row {
	t.Helper()
	d, err := store.GetDailyFigures(r, k)
	if err != nil {
		t.Fatal(err)
	}
	return render(d)
}

func TestDailyFigures(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "creationindex.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	b := s.NewBatch()
	apply(t, b, "bios-no-contribution", "gift.vaulta")
	apply(t, b, "genesis-batch", "gift.vaulta")
	apply(t, b, "buyram-pre-v330", "gift.vaulta")
	apply(t, b, "buyrambytes-pre-v330", "gift.vaulta")
	apply(t, b, "setacctram-outside-creation", "gift.vaulta")
	apply(t, b, "willsunshine", "gift.vaulta")
	if err := s.Commit(b); err != nil {
		t.Fatal(err)
	}
	b = s.NewBatch()
	apply(t, b, "reinaldo.gm", "gift.gm")
	if err := s.Commit(b); err != nil {
		t.Fatal(err)
	}

	genesis := row{Creations: 31, CreationsWithoutRAM: 1, Kinds: map[string]fig{
		"buy": {Contributions: 30, WithoutBytes: 30, RequestedBytes: 30 * 8192},
	}}
	buyram := row{Creations: 1, Kinds: map[string]fig{
		"buy": {Contributions: 1, WithoutBytes: 1, Quantity: "1.7900 EOS"},
	}}
	buyrambytes := row{Creations: 1, Kinds: map[string]fig{
		"buy": {Contributions: 1, WithoutBytes: 1, RequestedBytes: 1739},
	}}
	willsunshine := row{Creations: 1, Kinds: map[string]fig{
		"buy": {Contributions: 1, Bytes: 3299, Quantity: "1.1225 EOS"},
	}}
	reinaldo := row{Creations: 1, Kinds: map[string]fig{
		"program_gift": {Contributions: 1, Bytes: 4000},
	}}
	recent := row{Creations: 2, Kinds: map[string]fig{
		"buy":          {Contributions: 1, Bytes: 3299, Quantity: "1.1225 EOS"},
		"program_gift": {Contributions: 1, Bytes: 4000},
	}}

	network := map[string]row{
		"2018-06-09": genesis,
		"2018-07-10": buyram,
		"2023-08-12": buyrambytes,
		"2026-10-20": recent,
	}
	for d, want := range network {
		if got := getRow(t, s.DB(), store.DailyNetworkKey(day(t, d))); !reflect.DeepEqual(got, want) {
			t.Errorf("network %s:\ngot  %+v\nwant %+v", d, got, want)
		}
	}
	if got := countPrefix(t, s.DB(), []byte{store.DailyNetworkKey(0)[0]}); got != len(network) {
		t.Errorf("%d network rows, want %d", got, len(network))
	}

	creators := map[string]map[string]row{
		"2018-06-09": {"eosio": genesis},
		"2018-07-10": {"signupeoseos": buyram},
		"2023-08-12": {"genialwombat": buyrambytes},
		"2026-10-20": {"openvaccount": willsunshine, "gm": reinaldo},
	}
	total := 0
	for d, byCreator := range creators {
		dn := day(t, d)
		for c, want := range byCreator {
			cn := chain.StringToName(c)
			if got := getRow(t, s.DB(), store.DailyCreatorDayKey(cn, dn)); !reflect.DeepEqual(got, want) {
				t.Errorf("(creator, day) %s %s:\ngot  %+v\nwant %+v", c, d, got, want)
			}
			if got := getRow(t, s.DB(), store.DailyDayCreatorKey(dn, cn)); !reflect.DeepEqual(got, want) {
				t.Errorf("(day, creator) %s %s:\ngot  %+v\nwant %+v", d, c, got, want)
			}
		}
		if got := countPrefix(t, s.DB(), store.DailyDayCreatorPrefix(dn)); got != len(byCreator) {
			t.Errorf("%s has %d creators, want %d", d, got, len(byCreator))
		}
		total += len(byCreator)
	}
	if got := countPrefix(t, s.DB(), []byte{store.DailyCreatorDayKey(0, 0)[0]}); got != total {
		t.Errorf("%d (creator, day) rows, want %d", got, total)
	}
}

func countPrefix(t *testing.T, r pebble.Reader, prefix []byte) int {
	t.Helper()
	iter, err := r.NewIter(&pebble.IterOptions{LowerBound: prefix, UpperBound: store.PrefixEnd(prefix)})
	if err != nil {
		t.Fatal(err)
	}
	defer iter.Close()
	n := 0
	for iter.First(); iter.Valid(); iter.Next() {
		n++
	}
	return n
}

func TestQuantitySymbolMismatch(t *testing.T) {
	eos := chain.Asset{Amount: 10000, Symbol: chain.NewSymbolFromString(4, "EOS")}
	a := chain.Asset{Amount: 10000, Symbol: chain.NewSymbolFromString(4, "A")}
	d := &store.DailyFigures{}
	d.Kind(store.KindBuy).Quantity = eos
	add := &store.DailyFigures{}
	add.Kind(store.KindBuy).Quantity = a
	if err := Merge(d, add); err == nil {
		t.Fatal("merged EOS and A quantities")
	}
}
