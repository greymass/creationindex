package api

import (
	"cmp"
	"encoding/hex"
	"slices"
	"strconv"
	"time"

	"github.com/cockroachdb/pebble"
	"github.com/greymass/creationindex/internal/store"
	"github.com/greymass/roborovski/libraries/chain"
)

const (
	quotaWindowSeconds = 86400
	giftRowOverhead    = 136
)

var (
	actAddCreator = chain.StringToName("addcreator")
	actSetQuota   = chain.StringToName("setquota")
	actRmCreator  = chain.StringToName("rmcreator")
	actSuspend    = chain.StringToName("suspend")
	actResume     = chain.StringToName("resume")
	actSetAcctRAM = chain.StringToName("setacctram")
)

func secondsTime(sec int64) string {
	return time.Unix(sec, 0).UTC().Format(timeLayout)
}

// programCreatorOf joins a program gift to its giftacct; a top-up outside the giftee's creation has none.
func programCreatorOf(r pebble.Reader, e *store.ProgramEvent) (uint64, error) {
	if !e.InCreation {
		return 0, nil
	}
	g, err := store.GetGiftAcct(r, e.To)
	if err != nil || g == nil {
		return 0, err
	}
	return g.ProgramCreator, nil
}

func getProgramEvent(r pebble.Reader, seq uint64) (*store.ProgramEvent, error) {
	v, closer, err := r.Get(store.ProgramEventKey(seq))
	if err == pebble.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer closer.Close()
	return store.DecodeProgramEvent(v)
}

type programCreatorDayJSON struct {
	ProgramCreator  string         `json:"program_creator"`
	Gifts           uint64         `json:"gifts"`
	Accounts        uint64         `json:"accounts"`
	Bytes           int64          `json:"bytes"`
	BytesPerAccount map[string]int `json:"bytes_per_account"`
	accounts        map[uint64]bool
}

type programDayJSON struct {
	Day             string                   `json:"day"`
	ProgramCreators []*programCreatorDayJSON `json:"program_creators"`
}

type programDailyJSON struct {
	ProgramAccount string           `json:"program_account"`
	Days           []programDayJSON `json:"days"`
}

func (s *Server) getProgramDaily(r pebble.Reader, p params) (any, error) {
	from, to, err := p.dayRange()
	if err != nil {
		return nil, err
	}
	iter, err := r.NewIter(&pebble.IterOptions{
		LowerBound: store.ProgramGiftDayPrefix(from),
		UpperBound: store.ProgramGiftDayPrefix(to + 1),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	out := programDailyJSON{ProgramAccount: s.programAccount, Days: []programDayJSON{}}
	var day *programDayJSON
	byCreator := map[uint64]*programCreatorDayJSON{}
	for iter.First(); iter.Valid(); iter.Next() {
		d, seq, ok := store.DecodeProgramGiftDayKey(iter.Key())
		if !ok {
			return nil, store.ErrCorrupt
		}
		e, err := getProgramEvent(r, seq)
		if err != nil {
			return nil, err
		}
		if e == nil || e.Bytes == nil {
			return nil, store.ErrCorrupt
		}
		pc, err := programCreatorOf(r, e)
		if err != nil {
			return nil, err
		}
		if day == nil || day.Day != dayString(d) {
			out.Days = append(out.Days, programDayJSON{Day: dayString(d), ProgramCreators: []*programCreatorDayJSON{}})
			day = &out.Days[len(out.Days)-1]
			clear(byCreator)
		}
		row := byCreator[pc]
		if row == nil {
			row = &programCreatorDayJSON{ProgramCreator: name(pc), BytesPerAccount: map[string]int{}, accounts: map[uint64]bool{}}
			byCreator[pc] = row
			day.ProgramCreators = append(day.ProgramCreators, row)
		}
		row.Gifts++
		row.Bytes += *e.Bytes
		row.BytesPerAccount[strconv.FormatInt(*e.Bytes, 10)]++
		if !row.accounts[e.To] {
			row.accounts[e.To] = true
			row.Accounts++
		}
	}
	for i := range out.Days {
		slices.SortFunc(out.Days[i].ProgramCreators, func(a, b *programCreatorDayJSON) int {
			return cmp.Compare(a.ProgramCreator, b.ProgramCreator)
		})
	}
	return out, iter.Error()
}

type quotaJSON struct {
	Time            string `json:"time"`
	Action          string `json:"action"`
	DailyQuotaBytes int64  `json:"daily_quota_bytes"`
}

type suspensionJSON struct {
	Suspended string  `json:"suspended"`
	Resumed   *string `json:"resumed"`
}

type windowJSON struct {
	Start           string `json:"start"`
	End             string `json:"end"`
	Gifts           uint64 `json:"gifts"`
	UsedBytes       int64  `json:"used_bytes"`
	DailyQuotaBytes int64  `json:"daily_quota_bytes"`
	Open            bool   `json:"open"`
	EndedBy         string `json:"ended_by"`
	start           int64
}

type registrationJSON struct {
	Added       string           `json:"added"`
	Removed     *string          `json:"removed"`
	Quotas      []quotaJSON      `json:"quotas"`
	Suspensions []suspensionJSON `json:"suspensions"`
	Windows     []*windowJSON    `json:"windows"`
	quota       int64
}

type quotaHistoryJSON struct {
	ProgramCreator string              `json:"program_creator"`
	Registrations  []*registrationJSON `json:"registrations"`
}

// getQuota rebuilds quota windows with the contract's arithmetic (gift.cpp giftacct at 6fad4b0).
func (s *Server) getQuota(r pebble.Reader, p params) (any, error) {
	pc, ok, err := p.name("program_creator")
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, badRequest("program_creator is required")
	}
	prefix := store.ProgramCreatorIndexPrefix(pc)
	iter, err := r.NewIter(&pebble.IterOptions{LowerBound: prefix, UpperBound: store.PrefixEnd(prefix)})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	now := s.now().Unix()
	out := quotaHistoryJSON{ProgramCreator: name(pc), Registrations: []*registrationJSON{}}
	var reg *registrationJSON
	for iter.First(); iter.Valid(); iter.Next() {
		_, seq, ok := store.DecodeProgramCreatorIndexKey(iter.Key())
		if !ok {
			return nil, store.ErrCorrupt
		}
		entry, err := store.DecodeProgramCreatorEntry(iter.Value())
		if err != nil {
			return nil, err
		}
		if entry.Type == store.EntryGiftAcct {
			g, err := store.GetGiftAcct(r, entry.Account)
			if err != nil {
				return nil, err
			}
			if g == nil {
				return nil, store.ErrCorrupt
			}
			if reg == nil || reg.Removed != nil {
				continue
			}
			sec := store.BlockTimeUnixMilli(g.BlockTime) / 1000
			var w *windowJSON
			if n := len(reg.Windows); n > 0 && sec-reg.Windows[n-1].start < quotaWindowSeconds {
				w = reg.Windows[n-1]
			} else {
				w = &windowJSON{Start: secondsTime(sec), End: secondsTime(sec + quotaWindowSeconds), start: sec}
				reg.Windows = append(reg.Windows, w)
			}
			w.Gifts++
			w.UsedBytes += g.Bytes + giftRowOverhead
			w.DailyQuotaBytes = reg.quota
			continue
		}

		e, err := getProgramEvent(r, seq)
		if err != nil {
			return nil, err
		}
		if e == nil {
			return nil, store.ErrCorrupt
		}
		t := blockTime(e.BlockTime)
		switch e.Action {
		case actAddCreator:
			reg = &registrationJSON{Added: t, Quotas: []quotaJSON{}, Suspensions: []suspensionJSON{}, Windows: []*windowJSON{}}
			out.Registrations = append(out.Registrations, reg)
			fallthrough
		case actSetQuota:
			if reg == nil || reg.Removed != nil || e.DailyQuotaBytes == nil {
				continue
			}
			reg.quota = *e.DailyQuotaBytes
			reg.Quotas = append(reg.Quotas, quotaJSON{Time: t, Action: name(e.Action), DailyQuotaBytes: reg.quota})
		case actRmCreator:
			if reg == nil || reg.Removed != nil {
				continue
			}
			reg.Removed = &t
			if n := len(reg.Windows); n > 0 {
				w := reg.Windows[n-1]
				if rm := store.BlockTimeUnixMilli(e.BlockTime); rm < (w.start+quotaWindowSeconds)*1000 {
					w.End = t
					w.EndedBy = "rmcreator"
				}
			}
		case actSuspend:
			if reg != nil && reg.Removed == nil {
				reg.Suspensions = append(reg.Suspensions, suspensionJSON{Suspended: t})
			}
		case actResume:
			if reg != nil && reg.Removed == nil {
				if n := len(reg.Suspensions); n > 0 && reg.Suspensions[n-1].Resumed == nil {
					reg.Suspensions[n-1].Resumed = &t
				}
			}
		}
	}
	if n := len(out.Registrations); n > 0 {
		last := out.Registrations[n-1]
		if m := len(last.Windows); last.Removed == nil && m > 0 {
			w := last.Windows[m-1]
			w.Open = w.start+quotaWindowSeconds > now
		}
	}
	return out, iter.Error()
}

type programEventJSON struct {
	Type            string  `json:"type"`
	Action          string  `json:"action"`
	GlobalSeq       uint64  `json:"global_seq"`
	BlockNum        uint32  `json:"block_num"`
	BlockTime       string  `json:"block_time"`
	TrxID           string  `json:"trx_id"`
	ProgramCreator  string  `json:"program_creator"`
	From            string  `json:"from"`
	To              string  `json:"to"`
	Payer           string  `json:"payer"`
	Bytes           *int64  `json:"bytes"`
	AbsoluteQuota   bool    `json:"absolute_quota"`
	DailyQuotaBytes *int64  `json:"daily_quota_bytes"`
	Quantity        *string `json:"quantity"`
	Fee             *string `json:"fee"`
	Memo            string  `json:"memo"`
	InCreation      bool    `json:"in_creation"`
}

type programEventsJSON struct {
	Events []programEventJSON `json:"events"`
	More   bool               `json:"more"`
	Next   uint64             `json:"next"`
}

func encodeProgramEvent(r pebble.Reader, e *store.ProgramEvent) (programEventJSON, error) {
	out := programEventJSON{
		Type:            e.Type.String(),
		Action:          name(e.Action),
		GlobalSeq:       e.GlobalSeq,
		BlockNum:        e.BlockNum,
		BlockTime:       blockTime(e.BlockTime),
		TrxID:           hex.EncodeToString(e.TrxID[:]),
		From:            name(e.From),
		To:              name(e.To),
		Payer:           name(e.Payer),
		Bytes:           e.Bytes,
		AbsoluteQuota:   e.Action == actSetAcctRAM,
		DailyQuotaBytes: e.DailyQuotaBytes,
		Quantity:        assetString(e.Quantity),
		Fee:             assetString(e.Fee),
		Memo:            e.Memo,
		InCreation:      e.InCreation,
	}
	switch e.Type {
	case store.EventAdmin:
		out.ProgramCreator = name(e.Creator)
	case store.EventProgramGift:
		pc, err := programCreatorOf(r, e)
		if err != nil {
			return out, err
		}
		out.ProgramCreator = name(pc)
	}
	return out, nil
}

func (s *Server) getProgramEvents(r pebble.Reader, p params) (any, error) {
	var typ store.EventType
	if v := p["type"]; v != "" {
		for t := store.EventAdmin; t.Valid(); t++ {
			if t.String() == v {
				typ = t
			}
		}
		if typ == 0 {
			return nil, badRequest("type must be one of admin, program_gift, return, ram_movement")
		}
	}
	pc, byProgram, err := p.name("program_creator")
	if err != nil {
		return nil, err
	}
	after, err := p.after()
	if err != nil {
		return nil, err
	}
	limit, err := p.limit()
	if err != nil {
		return nil, err
	}

	var page []*store.ProgramEvent
	if byProgram {
		page, err = programEventsByProgramCreator(r, pc, typ, after, limit+1)
	} else {
		page, err = programEvents(r, typ, after, limit+1)
	}
	if err != nil {
		return nil, err
	}

	out := programEventsJSON{Events: []programEventJSON{}, Next: after}
	if len(page) > limit {
		out.More = true
		page = page[:limit]
	}
	for _, e := range page {
		j, err := encodeProgramEvent(r, e)
		if err != nil {
			return nil, err
		}
		out.Events = append(out.Events, j)
		out.Next = e.GlobalSeq
	}
	return out, nil
}

func programEvents(r pebble.Reader, typ store.EventType, after uint64, n int) ([]*store.ProgramEvent, error) {
	iter, err := r.NewIter(&pebble.IterOptions{
		LowerBound: store.ProgramEventKey(after + 1),
		UpperBound: store.PrefixEnd(store.ProgramEventPrefix()),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()
	var out []*store.ProgramEvent
	for iter.First(); iter.Valid() && len(out) < n; iter.Next() {
		e, err := store.DecodeProgramEvent(iter.Value())
		if err != nil {
			return nil, err
		}
		if typ == 0 || e.Type == typ {
			out = append(out, e)
		}
	}
	return out, iter.Error()
}

// A giftacct entry leads to the giftram the eosio stream wrote just after it in the same transaction.
func programEventsByProgramCreator(r pebble.Reader, pc uint64, typ store.EventType, after uint64, n int) ([]*store.ProgramEvent, error) {
	if typ != 0 && typ != store.EventAdmin && typ != store.EventProgramGift {
		return nil, nil
	}
	cursor, err := store.GetCursor(r, store.StreamEosio)
	if err != nil {
		return nil, err
	}
	iter, err := r.NewIter(&pebble.IterOptions{
		LowerBound: store.ProgramCreatorIndexKey(pc, after+1),
		UpperBound: store.PrefixEnd(store.ProgramCreatorIndexPrefix(pc)),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()
	var out []*store.ProgramEvent
	for iter.First(); iter.Valid() && len(out) < n; iter.Next() {
		_, seq, ok := store.DecodeProgramCreatorIndexKey(iter.Key())
		if !ok {
			return nil, store.ErrCorrupt
		}
		entry, err := store.DecodeProgramCreatorEntry(iter.Value())
		if err != nil {
			return nil, err
		}
		if entry.Type == store.EntryAdmin {
			if typ == store.EventProgramGift {
				continue
			}
			e, err := getProgramEvent(r, seq)
			if err != nil {
				return nil, err
			}
			if e == nil {
				return nil, store.ErrCorrupt
			}
			out = append(out, e)
			continue
		}
		if typ == store.EventAdmin {
			continue
		}
		if seq > cursor.GlobalSeq {
			break
		}
		e, err := giftOfGiftAcct(r, entry.Account, seq)
		if err != nil {
			return nil, err
		}
		if e != nil {
			out = append(out, e)
		}
	}
	return out, iter.Error()
}

func giftOfGiftAcct(r pebble.Reader, account, giftAcctSeq uint64) (*store.ProgramEvent, error) {
	g, err := store.GetGiftAcct(r, account)
	if err != nil || g == nil {
		return nil, err
	}
	iter, err := r.NewIter(&pebble.IterOptions{
		LowerBound: store.ProgramEventKey(giftAcctSeq + 1),
		UpperBound: store.PrefixEnd(store.ProgramEventPrefix()),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()
	for iter.First(); iter.Valid(); iter.Next() {
		e, err := store.DecodeProgramEvent(iter.Value())
		if err != nil {
			return nil, err
		}
		if e.BlockTime != g.BlockTime {
			return nil, nil
		}
		if e.Type == store.EventProgramGift && e.To == account && e.InCreation {
			return e, nil
		}
	}
	return nil, iter.Error()
}
