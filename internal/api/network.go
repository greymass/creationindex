package api

import (
	"cmp"
	"slices"

	"github.com/cockroachdb/pebble"
	"github.com/greymass/creationindex/internal/daily"
	"github.com/greymass/creationindex/internal/store"
)

func (s *Server) getCreation(r pebble.Reader, p params) (any, error) {
	account, ok, err := p.name("account")
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, badRequest("account is required")
	}
	c, err := store.GetCreation(r, account)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, notFound("no creation recorded for %s", p["account"])
	}
	return encodeCreation(r, c)
}

type creationsJSON struct {
	Creations []creationJSON `json:"creations"`
	More      bool           `json:"more"`
	Next      uint64         `json:"next"`
}

func (s *Server) getCreations(r pebble.Reader, p params) (any, error) {
	creator, byCreator, err := p.name("creator")
	if err != nil {
		return nil, err
	}
	programCreator, byProgram, err := p.name("program_creator")
	if err != nil {
		return nil, err
	}
	if byCreator == byProgram {
		return nil, badRequest("give one of creator or program_creator")
	}
	after, err := p.after()
	if err != nil {
		return nil, err
	}
	limit, err := p.limit()
	if err != nil {
		return nil, err
	}

	var page []*store.Creation
	if byCreator {
		page, err = creationsByCreator(r, creator, after, limit+1)
	} else {
		page, err = creationsByProgramCreator(r, programCreator, after, limit+1)
	}
	if err != nil {
		return nil, err
	}

	out := creationsJSON{Creations: []creationJSON{}, Next: after}
	if len(page) > limit {
		out.More = true
		page = page[:limit]
	}
	for _, c := range page {
		j, err := encodeCreation(r, c)
		if err != nil {
			return nil, err
		}
		out.Creations = append(out.Creations, j)
		out.Next = c.GlobalSeq
	}
	return out, nil
}

func creationsByCreator(r pebble.Reader, creator, after uint64, n int) ([]*store.Creation, error) {
	iter, err := r.NewIter(&pebble.IterOptions{
		LowerBound: store.CreatorIndexKey(creator, after+1),
		UpperBound: store.PrefixEnd(store.CreatorIndexPrefix(creator)),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()
	var out []*store.Creation
	for iter.First(); iter.Valid() && len(out) < n; iter.Next() {
		account, err := store.DecodeName(iter.Value())
		if err != nil {
			return nil, err
		}
		c, err := store.GetCreation(r, account)
		if err != nil {
			return nil, err
		}
		if c == nil {
			return nil, store.ErrCorrupt
		}
		out = append(out, c)
	}
	return out, iter.Error()
}

// creationsByProgramCreator seeks the giftacct index on after, which sits above
// each creation's own seq, so it skips entries whose creation is at or below after.
func creationsByProgramCreator(r pebble.Reader, programCreator, after uint64, n int) ([]*store.Creation, error) {
	cursor, err := store.GetCursor(r, store.StreamEosio)
	if err != nil {
		return nil, err
	}
	iter, err := r.NewIter(&pebble.IterOptions{
		LowerBound: store.ProgramCreatorIndexKey(programCreator, after+1),
		UpperBound: store.PrefixEnd(store.ProgramCreatorIndexPrefix(programCreator)),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()
	var out []*store.Creation
	for iter.First(); iter.Valid() && len(out) < n; iter.Next() {
		entry, err := store.DecodeProgramCreatorEntry(iter.Value())
		if err != nil {
			return nil, err
		}
		if entry.Type != store.EntryGiftAcct {
			continue
		}
		c, err := store.GetCreation(r, entry.Account)
		if err != nil {
			return nil, err
		}
		if c == nil {
			_, seq, _ := store.DecodeProgramCreatorIndexKey(iter.Key())
			if seq > cursor.GlobalSeq {
				break
			}
			continue
		}
		if c.GlobalSeq <= after {
			continue
		}
		out = append(out, c)
	}
	return out, iter.Error()
}

type dayJSON struct {
	Day                 string    `json:"day"`
	Creations           uint64    `json:"creations"`
	CreationsWithoutRAM uint64    `json:"creations_without_ram"`
	Kinds               kindsJSON `json:"kinds"`
}

type dailyJSON struct {
	Creator string    `json:"creator"`
	Days    []dayJSON `json:"days"`
}

func (s *Server) getDaily(r pebble.Reader, p params) (any, error) {
	creator, byCreator, err := p.name("creator")
	if err != nil {
		return nil, err
	}
	from, to, err := p.dayRange()
	if err != nil {
		return nil, err
	}
	lower, upper := store.DailyNetworkKey(from), store.DailyNetworkKey(to+1)
	if byCreator {
		lower, upper = store.DailyCreatorDayKey(creator, from), store.DailyCreatorDayKey(creator, to+1)
	}
	iter, err := r.NewIter(&pebble.IterOptions{LowerBound: lower, UpperBound: upper})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	out := dailyJSON{Creator: name(creator), Days: []dayJSON{}}
	for iter.First(); iter.Valid(); iter.Next() {
		var day uint32
		var ok bool
		if byCreator {
			_, day, ok = store.DecodeDailyCreatorDayKey(iter.Key())
		} else {
			day, ok = store.DecodeDailyNetworkKey(iter.Key())
		}
		if !ok {
			return nil, store.ErrCorrupt
		}
		d, err := store.DecodeDailyFigures(iter.Value())
		if err != nil {
			return nil, err
		}
		out.Days = append(out.Days, dayJSON{
			Day:                 dayString(day),
			Creations:           d.Creations,
			CreationsWithoutRAM: d.CreationsWithoutRAM,
			Kinds:               encodeKinds(d),
		})
	}
	return out, iter.Error()
}

type dayCreatorJSON struct {
	Creator             string    `json:"creator"`
	Creations           uint64    `json:"creations"`
	CreationsWithoutRAM uint64    `json:"creations_without_ram"`
	Kinds               kindsJSON `json:"kinds"`
}

type dayCreatorsJSON struct {
	Day      string           `json:"day"`
	Creators []dayCreatorJSON `json:"creators"`
	More     bool             `json:"more"`
	Offset   uint64           `json:"offset"`
}

type dayCreatorRow struct {
	creator uint64
	figures *store.DailyFigures
}

func (s *Server) getDayCreators(r pebble.Reader, p params) (any, error) {
	day, ok, err := p.day("day")
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, badRequest("day is required")
	}
	offset, err := p.uint("offset", 0)
	if err != nil {
		return nil, err
	}
	limit, err := p.limit()
	if err != nil {
		return nil, err
	}
	rows, err := sumDayCreators(r, day, day)
	if err != nil {
		return nil, err
	}
	creators, more, next := rankCreators(rows, offset, limit)
	return dayCreatorsJSON{Day: dayString(day), Creators: creators, More: more, Offset: next}, nil
}

type creatorsJSON struct {
	From     string           `json:"from"`
	To       string           `json:"to"`
	Creators []dayCreatorJSON `json:"creators"`
	More     bool             `json:"more"`
	Offset   uint64           `json:"offset"`
}

func (s *Server) getCreators(r pebble.Reader, p params) (any, error) {
	from, to, err := p.dayRange()
	if err != nil {
		return nil, err
	}
	offset, err := p.uint("offset", 0)
	if err != nil {
		return nil, err
	}
	limit, err := p.limit()
	if err != nil {
		return nil, err
	}
	rows, err := sumDayCreators(r, from, to)
	if err != nil {
		return nil, err
	}
	creators, more, next := rankCreators(rows, offset, limit)
	return creatorsJSON{From: dayString(from), To: dayString(to), Creators: creators, More: more, Offset: next}, nil
}

func sumDayCreators(r pebble.Reader, from, to uint32) ([]dayCreatorRow, error) {
	iter, err := r.NewIter(&pebble.IterOptions{
		LowerBound: store.DailyDayCreatorPrefix(from),
		UpperBound: store.PrefixEnd(store.DailyDayCreatorPrefix(to)),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()
	sums := map[uint64]*store.DailyFigures{}
	for iter.First(); iter.Valid(); iter.Next() {
		_, creator, ok := store.DecodeDailyDayCreatorKey(iter.Key())
		if !ok {
			return nil, store.ErrCorrupt
		}
		d, err := store.DecodeDailyFigures(iter.Value())
		if err != nil {
			return nil, err
		}
		if sum := sums[creator]; sum == nil {
			sums[creator] = d
		} else if err := daily.Merge(sum, d); err != nil {
			return nil, err
		}
	}
	if err := iter.Error(); err != nil {
		return nil, err
	}
	rows := make([]dayCreatorRow, 0, len(sums))
	for creator, d := range sums {
		rows = append(rows, dayCreatorRow{creator, d})
	}
	return rows, nil
}

func rankCreators(rows []dayCreatorRow, offset uint64, limit int) ([]dayCreatorJSON, bool, uint64) {
	slices.SortFunc(rows, func(a, b dayCreatorRow) int {
		return cmp.Or(cmp.Compare(b.figures.Creations, a.figures.Creations), cmp.Compare(a.creator, b.creator))
	})
	total := uint64(len(rows))
	start := min(offset, total)
	end := min(start+uint64(limit), total)
	out := []dayCreatorJSON{}
	for _, row := range rows[start:end] {
		out = append(out, dayCreatorJSON{
			Creator:             name(row.creator),
			Creations:           row.figures.Creations,
			CreationsWithoutRAM: row.figures.CreationsWithoutRAM,
			Kinds:               encodeKinds(row.figures),
		})
	}
	return out, end < total, end
}

type healthJSON struct {
	Status               string  `json:"status"`
	LastGlobalSeq        uint64  `json:"last_global_seq"`
	LastBlockTime        *string `json:"last_block_time"`
	ProgramLastGlobalSeq uint64  `json:"program_last_global_seq"`
	Creations            uint64  `json:"creations"`
	ProgramAccount       string  `json:"program_account"`
}

func (s *Server) health(r pebble.Reader, _ params) (any, error) {
	eosio, err := store.GetCursor(r, store.StreamEosio)
	if err != nil {
		return nil, err
	}
	gift, err := store.GetCursor(r, store.StreamGift)
	if err != nil {
		return nil, err
	}
	creations, err := store.GetCreationCount(r)
	if err != nil {
		return nil, err
	}
	out := healthJSON{
		Status:               "ok",
		LastGlobalSeq:        eosio.GlobalSeq,
		ProgramLastGlobalSeq: gift.GlobalSeq,
		Creations:            creations,
		ProgramAccount:       s.programAccount,
	}
	if eosio.GlobalSeq != 0 {
		t := blockTime(eosio.BlockTime)
		out.LastBlockTime = &t
	}
	return out, nil
}
