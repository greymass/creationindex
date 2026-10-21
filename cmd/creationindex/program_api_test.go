package main

import (
	"cmp"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/greymass/creationindex/internal/api"
	"github.com/greymass/creationindex/internal/store"
)

func TestGetProgramDailyMatchesPrototype(t *testing.T) {
	assertJSON(t, api.New(jungleStore(t), "gift.gm", ""), "GET", "/v1/creationindex/get_program_daily?from=2026-08-03&to=2026-10-20", "", 200, `{
		"program_account": "gift.gm",
		"days": [
			{"day": "2026-08-03", "program_creators": [{"program_creator": "", "gifts": 2, "accounts": 2, "bytes": 8000, "bytes_per_account": {"4000": 2}}]},
			{"day": "2026-10-18", "program_creators": [{"program_creator": "gm", "gifts": 3, "accounts": 3, "bytes": 58000, "bytes_per_account": {"4000": 2, "50000": 1}}]},
			{"day": "2026-10-19", "program_creators": [{"program_creator": "gm", "gifts": 6, "accounts": 6, "bytes": 254000, "bytes_per_account": {"4000": 1, "50000": 5}}]},
			{"day": "2026-10-20", "program_creators": [{"program_creator": "gm", "gifts": 2, "accounts": 2, "bytes": 8000, "bytes_per_account": {"4000": 2}}]}
		]
	}`)
	assertJSON(t, api.New(jungleStore(t), "gift.gm", ""), "GET", "/v1/creationindex/get_program_daily?from=2026-10-19&to=2026-10-19", "", 200, `{
		"program_account": "gift.gm",
		"days": [{"day": "2026-10-19", "program_creators": [{"program_creator": "gm", "gifts": 6, "accounts": 6, "bytes": 254000, "bytes_per_account": {"4000": 1, "50000": 5}}]}]
	}`)
	assertJSON(t, api.New(vaultaStore(t), "gift.vaulta", ""), "GET", "/v1/creationindex/get_program_daily?from=2026-10-01&to=2026-10-21", "", 200,
		`{"program_account": "gift.vaulta", "days": []}`)
}

func TestGetQuotaMatchesPrototype(t *testing.T) {
	j := api.New(jungleStore(t), "gift.gm", "")
	assertJSON(t, j, "GET", "/v1/creationindex/get_quota?program_creator=gm", "", 200, `{
		"program_creator": "gm",
		"registrations": [
			{
				"added": "2026-10-18T17:29:36.000Z", "removed": "2026-10-19T19:57:55.000Z",
				"quotas": [
					{"time": "2026-10-18T17:29:36.000Z", "action": "addcreator", "daily_quota_bytes": 10000},
					{"time": "2026-10-18T18:56:52.500Z", "action": "setquota", "daily_quota_bytes": 1000000}
				],
				"suspensions": [],
				"windows": [
					{"start": "2026-10-18T18:16:07.000Z", "end": "2026-10-19T18:16:07.000Z", "gifts": 5, "used_bytes": 158680, "daily_quota_bytes": 1000000, "open": false, "ended_by": ""},
					{"start": "2026-10-19T18:47:28.000Z", "end": "2026-10-19T19:57:55.000Z", "gifts": 2, "used_bytes": 100272, "daily_quota_bytes": 1000000, "open": false, "ended_by": "rmcreator"}
				]
			},
			{
				"added": "2026-10-19T19:58:38.000Z", "removed": null,
				"quotas": [{"time": "2026-10-19T19:58:38.000Z", "action": "addcreator", "daily_quota_bytes": 262144}],
				"suspensions": [{"suspended": "2026-10-19T20:13:46.000Z", "resumed": "2026-10-19T20:16:42.000Z"}],
				"windows": [
					{"start": "2026-10-19T20:00:06.000Z", "end": "2026-10-20T20:00:06.000Z", "gifts": 4, "used_bytes": 62544, "daily_quota_bytes": 262144, "open": false, "ended_by": ""}
				]
			}
		]
	}`)
	assertJSON(t, j, "GET", "/v1/creationindex/get_quota?program_creator=nobody", "", 200, `{"program_creator": "nobody", "registrations": []}`)
	assertJSON(t, j, "GET", "/v1/creationindex/get_quota", "", 400, `{"error": "program_creator is required"}`)
}

func TestGetProgramEventsMatchesPrototype(t *testing.T) {
	j := api.New(jungleStore(t), "gift.gm", "")
	assertJSON(t, j, "GET", "/v1/creationindex/get_program_events?type=return", "", 200, `{
		"events": [{
			"type": "return", "action": "ungiftram", "global_seq": 396535542, "block_num": 292925796, "block_time": "2026-10-19T18:57:56.000Z",
			"trx_id": "1ddef957020db81cbf97254e7d28af1832a5367ee3a7c53ff331524fc095e631",
			"program_creator": "", "from": "grbweziu.gm", "to": "gift.gm", "payer": "", "bytes": 50000, "absolute_quota": false,
			"daily_quota_bytes": null, "quantity": null, "fee": null, "memo": "return faucet gift before isolated source test", "in_creation": false
		}],
		"more": false, "next": 396535542
	}`)
	assertJSON(t, j, "GET", "/v1/creationindex/get_program_events?type=ram_movement", "", 200, `{
		"events": [
			{
				"type": "ram_movement", "action": "buyrambytes", "global_seq": 365280805, "block_num": 279623247, "block_time": "2026-08-03T18:45:15.000Z",
				"trx_id": "8b2ebda82c519d610455e6fc6d23f0f2b7ca79eee409ab1f3df4fd62d7cbab03",
				"program_creator": "", "from": "", "to": "gift.gm", "payer": "gm", "bytes": 499999, "absolute_quota": false,
				"daily_quota_bytes": null, "quantity": "102.1851 EOS", "fee": "0.5110 EOS", "memo": "", "in_creation": true
			},
			{
				"type": "ram_movement", "action": "ramtransfer", "global_seq": 365287401, "block_num": 279625088, "block_time": "2026-08-03T19:00:35.500Z",
				"trx_id": "2e4c9ea1473160868db1fffef9ec394cf919228862e4777ba16d2b51cd7e8cb1",
				"program_creator": "", "from": "gm", "to": "gift.gm", "payer": "", "bytes": 1000000, "absolute_quota": false,
				"daily_quota_bytes": null, "quantity": null, "fee": null, "memo": "", "in_creation": false
			}
		],
		"more": false, "next": 365287401
	}`)
	assertJSON(t, j, "GET", "/v1/creationindex/get_program_events?type=return&program_creator=gm", "", 200, `{"events": [], "more": false, "next": 0}`)
	assertJSON(t, j, "GET", "/v1/creationindex/get_program_events?type=gift", "", 400, `{"error": "type must be one of admin, program_gift, return, ram_movement"}`)
	assertJSON(t, api.New(vaultaStore(t), "gift.vaulta", ""), "GET", "/v1/creationindex/get_program_events", "", 200, `{"events": [], "more": false, "next": 0}`)
}

type eventRef struct {
	Type           string `json:"type"`
	Action         string `json:"action"`
	GlobalSeq      uint64 `json:"global_seq"`
	ProgramCreator string `json:"program_creator"`
	To             string `json:"to"`
}

func pageEvents(t *testing.T, h http.Handler, path string) ([]eventRef, bool, uint64) {
	t.Helper()
	code, body := call(t, h, "GET", path, "")
	if code != 200 {
		t.Fatalf("%s: %d %s", path, code, body)
	}
	var out struct {
		Events []eventRef `json:"events"`
		More   bool       `json:"more"`
		Next   uint64     `json:"next"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	return out.Events, out.More, out.Next
}

func countTypes(events []eventRef) map[string]int {
	n := map[string]int{}
	for _, e := range events {
		n[e.Type]++
	}
	return n
}

func TestGetProgramEventsPaging(t *testing.T) {
	j := api.New(jungleStore(t), "gift.gm", "")
	for _, base := range []string{"/v1/creationindex/get_program_events?", "/v1/creationindex/get_program_events?program_creator=gm"} {
		whole, more, _ := pageEvents(t, j, base+"&limit=1000")
		if more {
			t.Fatalf("%s: more on the whole list", base)
		}
		var paged []eventRef
		var after uint64
		for {
			page, more, next := pageEvents(t, j, fmt.Sprintf("%s&limit=3&after=%d", base, after))
			paged = append(paged, page...)
			after = next
			if !more {
				break
			}
		}
		if !slices.Equal(whole, paged) {
			t.Errorf("%s: paged %v, whole %v", base, paged, whole)
		}
		if !slices.IsSortedFunc(whole, func(a, b eventRef) int { return cmp.Compare(a.GlobalSeq, b.GlobalSeq) }) {
			t.Errorf("%s: not in seq order", base)
		}
	}

	all, _, _ := pageEvents(t, j, "/v1/creationindex/get_program_events?limit=1000")
	if got := countTypes(all); got["admin"] != 7 || got["program_gift"] != 13 || got["return"] != 1 || got["ram_movement"] != 2 {
		t.Errorf("all events by type %v", got)
	}

	gm, _, _ := pageEvents(t, j, "/v1/creationindex/get_program_events?program_creator=gm&limit=1000")
	if got := countTypes(gm); got["admin"] != 6 || got["program_gift"] != 11 || len(got) != 2 {
		t.Errorf("gm events by type %v", got)
	}
	for _, e := range gm {
		if e.ProgramCreator != "gm" {
			t.Errorf("event %d has program_creator %q", e.GlobalSeq, e.ProgramCreator)
		}
	}
	gifts, _, _ := pageEvents(t, j, "/v1/creationindex/get_program_events?program_creator=gm&type=program_gift&limit=1000")
	if len(gifts) != 11 || gifts[len(gifts)-1].To != "reinaldo.gm" || gifts[len(gifts)-1].GlobalSeq != 396724550 {
		t.Errorf("gm program gifts %v", gifts)
	}
	unattributed, _, _ := pageEvents(t, j, "/v1/creationindex/get_program_events?type=program_gift&limit=1000")
	n := 0
	for _, e := range unattributed {
		if e.ProgramCreator == "" {
			n++
		}
	}
	if n != 2 {
		t.Errorf("%d program gifts without a program creator, want the 2 pre-redeploy gifts", n)
	}
}

func TestGetEndowment(t *testing.T) {
	var calls atomic.Int32
	chain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var req struct {
			AccountName string `json:"account_name"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if r.URL.Path != "/v1/chain/get_account" || req.AccountName != "gift.gm" {
			w.WriteHeader(400)
			fmt.Fprint(w, `{"code":400,"message":"Account lookup","error":{"code":3060002,"name":"account_query_exception"}}`)
			return
		}
		fmt.Fprint(w, `{"account_name":"gift.gm","ram_quota":1223399,"ram_usage":217768,"net_weight":1000000}`)
	}))
	defer chain.Close()

	j := api.New(jungleStore(t), "gift.gm", chain.URL+"/")
	for range 3 {
		code, body := call(t, j, "GET", "/v1/creationindex/get_endowment", "")
		var out map[string]any
		if err := json.Unmarshal([]byte(body), &out); err != nil || code != 200 {
			t.Fatalf("%d %s", code, body)
		}
		if out["program_account"] != "gift.gm" || out["ram_quota"] != 1223399.0 || out["ram_usage"] != 217768.0 || out["available_bytes"] != 1005631.0 || out["read_at"] == "" {
			t.Errorf("endowment %s", body)
		}
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("%d chain reads for 3 requests within the cache", n)
	}

	v := api.New(vaultaStore(t), "gift.vaulta", chain.URL)
	assertJSON(t, v, "GET", "/v1/creationindex/get_endowment", "", 404, `{"error": "program account gift.vaulta does not exist"}`)
	assertJSON(t, api.New(vaultaStore(t), "gift.vaulta", ""), "GET", "/v1/creationindex/get_endowment", "", 503, `{"error": "chain-url is not set"}`)
	down := api.New(vaultaStore(t), "gift.vaulta", "http://127.0.0.1:1")
	if code, _ := call(t, down, "GET", "/v1/creationindex/get_endowment", ""); code != 502 {
		t.Errorf("unreachable chain: status %d, want 502", code)
	}
}

func TestGetProgramEventsByProgramCreatorWaitsForEosioStream(t *testing.T) {
	db := jungleStore(t)
	b := db.NewBatch()
	if err := store.SetCursor(b, store.StreamEosio, store.Cursor{GlobalSeq: 396333030}); err != nil {
		t.Fatal(err)
	}
	if err := b.Delete(store.ProgramEventKey(396333061), nil); err != nil {
		t.Fatal(err)
	}
	if err := db.Commit(b); err != nil {
		t.Fatal(err)
	}
	got, more, next := pageEvents(t, api.New(db, "gift.gm", ""), "/v1/creationindex/get_program_events?program_creator=gm")
	if len(got) != 2 || got[0].Action != "addcreator" || got[1].GlobalSeq != 396333029 || more || next != 396333029 {
		t.Errorf("%v more=%v next=%d; want the page to stop where the eosio stream has not reached", got, more, next)
	}
}
