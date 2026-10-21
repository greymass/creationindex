package main

import (
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/greymass/creationindex/internal/api"
	"github.com/greymass/creationindex/internal/program"
	"github.com/greymass/creationindex/internal/store"
	"github.com/greymass/roborovski/libraries/actionstream"
	"github.com/greymass/roborovski/libraries/chain"
)

type fixture struct {
	pkg, name string
}

func buildStore(t *testing.T, programAccount string, eosio []fixture, gift []fixture) *store.Store {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "creationindex.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	var stream []actionstream.Action
	for _, f := range eosio {
		stream = append(stream, loadFixtureFrom(t, f.pkg, f.name)...)
	}
	slices.SortStableFunc(stream, func(a, b actionstream.Action) int { return cmp.Compare(a.GlobalSeq, b.GlobalSeq) })
	b := db.NewBatch()
	var buf txBuffer
	apply := func(tx []actionstream.Action) {
		if len(tx) == 0 {
			return
		}
		if _, err := applyEosioTransaction(b, tx, chain.StringToName(programAccount)); err != nil {
			t.Fatal(err)
		}
		last := tx[len(tx)-1]
		if err := store.SetCursor(b, store.StreamEosio, store.Cursor{GlobalSeq: last.GlobalSeq, BlockTime: last.BlockTime}); err != nil {
			t.Fatal(err)
		}
	}
	for _, a := range stream {
		apply(buf.push(a))
	}
	apply(buf.take())

	g := program.NewGiftStream(programAccount)
	for _, f := range gift {
		actions := loadFixtureFrom(t, f.pkg, f.name)
		for i := range actions {
			if _, err := g.Apply(b, &actions[i]); err != nil {
				t.Fatal(err)
			}
			if err := store.SetCursor(b, store.StreamGift, store.Cursor{GlobalSeq: actions[i].GlobalSeq, BlockTime: actions[i].BlockTime}); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := db.Commit(b); err != nil {
		t.Fatal(err)
	}
	return db
}

func vaultaStore(t *testing.T) *store.Store {
	var eosio []fixture
	for _, n := range []string{"aaaisn.b.sat", "bios-no-contribution", "buyram-pre-v330", "buyrambytes-pre-v330", "buyrambytes-v330-no-fee", "genesis-batch", "ramtransfer-wrapped", "setacctram-outside-creation", "willsunshine"} {
		eosio = append(eosio, fixture{"attribution", n})
	}
	return buildStore(t, "gift.vaulta", eosio, nil)
}

func jungleStore(t *testing.T) *store.Store {
	return buildStore(t, "gift.gm",
		[]fixture{{"program", "gift-gm-eosio"}},
		[]fixture{{"program", "gift-gm"}})
}

func call(t *testing.T, h http.Handler, method, path, body string) (int, string) {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, r))
	return rec.Code, rec.Body.String()
}

func assertJSON(t *testing.T, h http.Handler, method, path, body string, wantCode int, want string) {
	t.Helper()
	code, got := call(t, h, method, path, body)
	if code != wantCode {
		t.Errorf("%s %s: status %d, want %d: %s", method, path, code, wantCode, got)
	}
	var g, w any
	if err := json.Unmarshal([]byte(got), &g); err != nil {
		t.Fatalf("%s %s: %v: %s", method, path, err, got)
	}
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(g, w) {
		t.Errorf("%s %s:\n got %s\nwant %s", method, path, got, want)
	}
}

const zeroKind = `{"contributions": 0, "bytes": null, "without_bytes": 0, "requested_bytes": null, "quantity": null}`

func TestGetCreationMatchesPrototype(t *testing.T) {
	v := api.New(vaultaStore(t), "gift.vaulta", "")
	j := api.New(jungleStore(t), "gift.gm", "")

	assertJSON(t, v, "GET", "/v1/creationindex/get_creation?account=willsunshine", "", 200, `{
		"account": "willsunshine", "creator": "openvaccount", "program_creator": "",
		"block_num": 526448146, "block_time": "2026-10-20T22:29:07.000Z",
		"trx_id": "3efce2e5432e2e1ce595615e19fa245118e6766414022bc935a7a8eebfa79258",
		"global_seq": 368360304290,
		"contributions": [{"kind": "buy", "action": "buyrambytes", "global_seq": 368360304296, "payer": "openvaccount",
			"bytes": 3299, "requested_bytes": null, "quantity": "1.1225 EOS", "fee": "0.0057 EOS", "memo": ""}]
	}`)
	assertJSON(t, v, "POST", "/v1/creationindex/get_creation", `{"account": "aaaisn.b.sat"}`, 200, `{
		"account": "aaaisn.b.sat", "creator": "sat", "program_creator": "",
		"block_num": 526056215, "block_time": "2026-10-18T16:01:10.500Z",
		"trx_id": "51913702421ccde6dc815fa9e65fe16a3ae21345125438717a511fd00bfa160b",
		"global_seq": 368348901051,
		"contributions": [{"kind": "gift", "action": "giftram", "global_seq": 368348901052, "payer": "signup.xsat",
			"bytes": 6144, "requested_bytes": null, "quantity": null, "fee": null, "memo": "Bank account signup"}]
	}`)
	assertJSON(t, j, "GET", "/v1/creationindex/get_creation?account=reinaldo.gm", "", 200, `{
		"account": "reinaldo.gm", "creator": "gm", "program_creator": "gm",
		"block_num": 293072493, "block_time": "2026-10-20T15:21:32.500Z",
		"trx_id": "071ea115a6f4ceb339cb873cd5710b5e874ee8c83050f82c59bf96910c06d7e5",
		"global_seq": 396724548,
		"contributions": [{"kind": "program_gift", "action": "giftram", "global_seq": 396724550, "payer": "gift.gm",
			"bytes": 4000, "requested_bytes": null, "quantity": null, "fee": null, "memo": ""}]
	}`)
	assertJSON(t, v, "GET", "/v1/creationindex/get_creation?account=eosio.msig", "", 200, `{
		"account": "eosio.msig", "creator": "eosio", "program_creator": "",
		"block_num": 132, "block_time": "2018-06-09T11:57:35.000Z",
		"trx_id": "708f08725f1e7f86670a3e4f5e01f8833df875b5949513643a0a362e3c6ac75f",
		"global_seq": 134, "contributions": []
	}`)
}

func TestRequestErrors(t *testing.T) {
	v := api.New(vaultaStore(t), "gift.vaulta", "")
	for _, c := range []struct {
		method, path, body string
		code               int
		msg                string
	}{
		{"GET", "/v1/creationindex/get_creation?account=nosuchacct", "", 404, "no creation recorded for nosuchacct"},
		{"GET", "/v1/creationindex/get_creation", "", 400, "account is required"},
		{"GET", "/v1/creationindex/get_creation?account=Bad", "", 400, "account is not a valid account name"},
		{"POST", "/v1/creationindex/get_creation", `{"account": ["a"]}`, 400, "account must be a string or a number"},
		{"POST", "/v1/creationindex/get_creation", `not json`, 400, "request body must be a JSON object"},
		{"GET", "/v1/creationindex/get_creations", "", 400, "give one of creator or program_creator"},
		{"GET", "/v1/creationindex/get_creations?creator=eosio&program_creator=gm", "", 400, "give one of creator or program_creator"},
		{"GET", "/v1/creationindex/get_creations?creator=eosio&limit=1001", "", 400, "limit must be between 1 and 1000"},
		{"GET", "/v1/creationindex/get_creations?creator=eosio&after=-1", "", 400, "after must be a non-negative integer"},
		{"GET", "/v1/creationindex/get_creations?creator=eosio&after=18446744073709551615", "", 400, "after is out of range"},
		{"GET", "/v1/creationindex/get_daily?from=2026-10-20", "", 400, "to is required"},
		{"GET", "/v1/creationindex/get_daily?from=2026-10-20&to=2026-10-19", "", 400, "to is before from"},
		{"GET", "/v1/creationindex/get_daily?from=2025-10-20&to=2026-10-21", "", 400, "at most 366 days per request"},
		{"GET", "/v1/creationindex/get_daily?from=20261020&to=2026-10-21", "", 400, "from must be a day as YYYY-MM-DD"},
		{"GET", "/v1/creationindex/get_day_creators", "", 400, "day is required"},
		{"GET", "/v1/creationindex/get_nothing", "", 404, "not found"},
		{"DELETE", "/v1/creationindex/health", "", 405, "method not allowed"},
	} {
		assertJSON(t, v, c.method, c.path, c.body, c.code, `{"error": "`+c.msg+`"}`)
	}
	if code, _ := call(t, v, "GET", "/v1/creationindex/get_daily?from=2025-10-21&to=2026-10-21", ""); code != 200 {
		t.Errorf("366 days: status %d", code)
	}
	if code, _ := call(t, v, "OPTIONS", "/v1/creationindex/health", ""); code != 204 {
		t.Errorf("OPTIONS: status %d", code)
	}
}

func pageAccounts(t *testing.T, h http.Handler, path string) ([]string, bool, uint64) {
	t.Helper()
	code, body := call(t, h, "GET", path, "")
	if code != 200 {
		t.Fatalf("%s: %d %s", path, code, body)
	}
	var page struct {
		Creations []struct {
			Account   string `json:"account"`
			GlobalSeq uint64 `json:"global_seq"`
		} `json:"creations"`
		More bool   `json:"more"`
		Next uint64 `json:"next"`
	}
	if err := json.Unmarshal([]byte(body), &page); err != nil {
		t.Fatal(err)
	}
	var accounts []string
	for _, c := range page.Creations {
		accounts = append(accounts, c.Account)
	}
	return accounts, page.More, page.Next
}

func TestGetCreationsPaging(t *testing.T) {
	v := api.New(vaultaStore(t), "gift.vaulta", "")
	j := api.New(jungleStore(t), "gift.gm", "")

	got, more, next := pageAccounts(t, j, "/v1/creationindex/get_creations?program_creator=gm&limit=2")
	if !slices.Equal(got, []string{"ssyfp5xf.gm", "btvxpqb4.gm"}) || !more || next != 396333059 {
		t.Errorf("program_creator=gm first page %v more=%v next=%d", got, more, next)
	}

	var all []string
	var after uint64
	for pages := 0; ; pages++ {
		got, more, next := pageAccounts(t, j, fmt.Sprintf("/v1/creationindex/get_creations?program_creator=gm&limit=3&after=%d", after))
		all = append(all, got...)
		if !more {
			break
		}
		if pages > 10 || next <= after {
			t.Fatalf("paging did not advance: after=%d next=%d", after, next)
		}
		after = next
	}
	whole, more, _ := pageAccounts(t, j, "/v1/creationindex/get_creations?program_creator=gm")
	if more || len(whole) == 0 || !slices.Equal(all, whole) {
		t.Errorf("paged %v, whole %v (more=%v)", all, whole, more)
	}
	if !slices.Contains(whole, "reinaldo.gm") {
		t.Errorf("reinaldo.gm missing from %v", whole)
	}

	got, more, next = pageAccounts(t, j, "/v1/creationindex/get_creations?program_creator=gm&after=396333027&limit=1")
	if !slices.Equal(got, []string{"btvxpqb4.gm"}) || !more || next != 396333059 {
		t.Errorf("after the first creation's seq: %v more=%v next=%d", got, more, next)
	}

	got, more, next = pageAccounts(t, v, "/v1/creationindex/get_creations?creator=eosio&limit=30")
	if len(got) != 30 || got[0] != "eosio.msig" || !more {
		t.Fatalf("eosio first page %v more=%v", got, more)
	}
	rest, more, last := pageAccounts(t, v, fmt.Sprintf("/v1/creationindex/get_creations?creator=eosio&after=%d", next))
	if len(rest) != 1 || more || last != 921 {
		t.Errorf("eosio second page %v more=%v next=%d", rest, more, last)
	}
	empty, more, same := pageAccounts(t, v, "/v1/creationindex/get_creations?creator=eosio&after=921")
	if len(empty) != 0 || more || same != 921 {
		t.Errorf("past the end: %v more=%v next=%d", empty, more, same)
	}
	assertJSON(t, v, "GET", "/v1/creationindex/get_creations?program_creator=gm", "", 200, `{"creations": [], "more": false, "next": 0}`)
}

func TestGetCreationsByProgramCreatorWaitsForEosioStream(t *testing.T) {
	db := jungleStore(t)
	b := db.NewBatch()
	if err := store.SetCursor(b, store.StreamEosio, store.Cursor{GlobalSeq: 396333030}); err != nil {
		t.Fatal(err)
	}
	if err := b.Delete(store.CreationKey(chain.StringToName("btvxpqb4.gm")), nil); err != nil {
		t.Fatal(err)
	}
	if err := db.Commit(b); err != nil {
		t.Fatal(err)
	}
	got, more, next := pageAccounts(t, api.New(db, "gift.gm", ""), "/v1/creationindex/get_creations?program_creator=gm")
	if !slices.Equal(got, []string{"ssyfp5xf.gm"}) || more || next != 396333027 {
		t.Errorf("%v more=%v next=%d; want the page to stop where the eosio stream has not reached", got, more, next)
	}
}

func TestGetDaily(t *testing.T) {
	v := api.New(vaultaStore(t), "gift.vaulta", "")
	genesis := `{
		"day": "2018-06-09", "creations": 31, "creations_without_ram": 1,
		"kinds": {
			"buy": {"contributions": 30, "bytes": null, "without_bytes": 30, "requested_bytes": 245760, "quantity": null},
			"transfer": ` + zeroKind + `, "gift": ` + zeroKind + `, "program_gift": ` + zeroKind + `, "quota_set": ` + zeroKind + `
		}
	}`
	assertJSON(t, v, "GET", "/v1/creationindex/get_daily?from=2018-06-08&to=2018-06-10", "", 200, `{"creator": "", "days": [`+genesis+`]}`)
	assertJSON(t, v, "POST", "/v1/creationindex/get_daily", `{"creator": "eosio", "from": "2018-06-09", "to": "2018-06-09"}`, 200, `{"creator": "eosio", "days": [`+genesis+`]}`)
	assertJSON(t, v, "GET", "/v1/creationindex/get_daily?creator=openvaccount&from=2026-10-20&to=2026-10-20", "", 200, `{
		"creator": "openvaccount",
		"days": [{
			"day": "2026-10-20", "creations": 1, "creations_without_ram": 0,
			"kinds": {
				"buy": {"contributions": 1, "bytes": 3299, "without_bytes": 0, "requested_bytes": null, "quantity": "1.1225 EOS"},
				"transfer": `+zeroKind+`, "gift": `+zeroKind+`, "program_gift": `+zeroKind+`, "quota_set": `+zeroKind+`
			}
		}]
	}`)
	assertJSON(t, v, "GET", "/v1/creationindex/get_daily?creator=openvaccount&from=2026-10-21&to=2026-10-22", "", 200, `{"creator": "openvaccount", "days": []}`)
}

func TestGetDayCreatorsRanksAndPages(t *testing.T) {
	db := vaultaStore(t)
	day := store.DayFromBlockTime(1)
	b := db.NewBatch()
	for _, r := range []struct {
		creator   string
		creations uint64
	}{{"carol", 2}, {"alice", 5}, {"bob", 2}, {"dave", 1}} {
		if err := store.SetDailyFigures(b, store.DailyDayCreatorKey(day, chain.StringToName(r.creator)), &store.DailyFigures{Creations: r.creations}); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Commit(b); err != nil {
		t.Fatal(err)
	}
	v := api.New(db, "gift.vaulta", "")

	rank := func(path string) ([]string, bool, uint64) {
		code, body := call(t, v, "GET", path, "")
		if code != 200 {
			t.Fatalf("%s: %d %s", path, code, body)
		}
		var out struct {
			Creators []struct {
				Creator string `json:"creator"`
			} `json:"creators"`
			More   bool   `json:"more"`
			Offset uint64 `json:"offset"`
		}
		if err := json.Unmarshal([]byte(body), &out); err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, c := range out.Creators {
			names = append(names, c.Creator)
		}
		return names, out.More, out.Offset
	}
	if got, more, off := rank("/v1/creationindex/get_day_creators?day=2000-01-01&limit=2"); !slices.Equal(got, []string{"alice", "bob"}) || !more || off != 2 {
		t.Errorf("first page %v more=%v offset=%d", got, more, off)
	}
	if got, more, off := rank("/v1/creationindex/get_day_creators?day=2000-01-01&limit=2&offset=2"); !slices.Equal(got, []string{"carol", "dave"}) || more || off != 4 {
		t.Errorf("second page %v more=%v offset=%d", got, more, off)
	}
	if got, more, off := rank("/v1/creationindex/get_day_creators?day=2000-01-01&offset=9"); len(got) != 0 || more || off != 4 {
		t.Errorf("past the end %v more=%v offset=%d", got, more, off)
	}
	assertJSON(t, v, "GET", "/v1/creationindex/get_day_creators?day=2018-06-09", "", 200, `{
		"day": "2018-06-09",
		"creators": [{
			"creator": "eosio", "creations": 31, "creations_without_ram": 1,
			"kinds": {
				"buy": {"contributions": 30, "bytes": null, "without_bytes": 30, "requested_bytes": 245760, "quantity": null},
				"transfer": `+zeroKind+`, "gift": `+zeroKind+`, "program_gift": `+zeroKind+`, "quota_set": `+zeroKind+`
			}
		}],
		"more": false, "offset": 1
	}`)
}

func TestHealth(t *testing.T) {
	assertJSON(t, api.New(vaultaStore(t), "gift.vaulta", ""), "GET", "/v1/creationindex/health", "", 200, `{
		"status": "ok", "last_global_seq": 368360304310, "last_block_time": "2026-10-20T22:29:07.000Z",
		"program_last_global_seq": 0, "creations": 37, "program_account": "gift.vaulta"
	}`)
	assertJSON(t, api.New(jungleStore(t), "gift.gm", ""), "GET", "/v1/creationindex/health", "", 200, `{
		"status": "ok", "last_global_seq": 396724550, "last_block_time": "2026-10-20T15:21:32.500Z",
		"program_last_global_seq": 396724549, "creations": 14, "program_account": "gift.gm"
	}`)
	empty, err := store.Open(filepath.Join(t.TempDir(), "empty.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer empty.Close()
	assertJSON(t, api.New(empty, "gift.vaulta", ""), "GET", "/v1/creationindex/health", "", 200, `{
		"status": "ok", "last_global_seq": 0, "last_block_time": null,
		"program_last_global_seq": 0, "creations": 0, "program_account": "gift.vaulta"
	}`)
}
