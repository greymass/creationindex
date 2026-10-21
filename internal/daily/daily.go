package daily

import (
	"fmt"

	"github.com/cockroachdb/pebble"
	"github.com/greymass/creationindex/internal/store"
	"github.com/greymass/roborovski/libraries/chain"
)

func Add(d *store.DailyFigures, c *store.Creation) error {
	d.Creations++
	if len(c.Contributions) == 0 {
		d.CreationsWithoutRAM++
	}
	for i := range c.Contributions {
		k := &c.Contributions[i]
		if !k.Kind.Valid() {
			return fmt.Errorf("daily: %s contribution kind %d", chain.NameToString(c.Account), k.Kind)
		}
		f := d.Kind(k.Kind)
		f.Contributions++
		if k.Bytes == nil {
			f.WithoutBytes++
		} else if *k.Bytes < 0 {
			return fmt.Errorf("daily: %s contribution at seq %d has negative bytes %d", chain.NameToString(c.Account), k.GlobalSeq, *k.Bytes)
		} else {
			f.Bytes += uint64(*k.Bytes)
		}
		if k.RequestedBytes != nil {
			if *k.RequestedBytes < 0 {
				return fmt.Errorf("daily: %s contribution at seq %d has negative requested bytes %d", chain.NameToString(c.Account), k.GlobalSeq, *k.RequestedBytes)
			}
			f.RequestedBytes += uint64(*k.RequestedBytes)
		}
		if k.Quantity != nil {
			if err := addAsset(&f.Quantity, *k.Quantity); err != nil {
				return fmt.Errorf("daily: %s contribution at seq %d: %w", chain.NameToString(c.Account), k.GlobalSeq, err)
			}
		}
	}
	return nil
}

func Merge(dst, src *store.DailyFigures) error {
	dst.Creations += src.Creations
	dst.CreationsWithoutRAM += src.CreationsWithoutRAM
	for i := range dst.Kinds {
		d, s := &dst.Kinds[i], &src.Kinds[i]
		d.Contributions += s.Contributions
		d.Bytes += s.Bytes
		d.WithoutBytes += s.WithoutBytes
		d.RequestedBytes += s.RequestedBytes
		if err := addAsset(&d.Quantity, s.Quantity); err != nil {
			return fmt.Errorf("daily: %w", err)
		}
	}
	return nil
}

// addAsset treats symbol 0 as an empty sum that takes the other side's symbol.
func addAsset(sum *chain.Asset, v chain.Asset) error {
	switch {
	case v.Symbol == 0 && v.Amount == 0:
		return nil
	case sum.Symbol == 0 && sum.Amount == 0:
		*sum = v
		return nil
	case sum.Symbol != v.Symbol:
		return fmt.Errorf("quantity %s does not match the sum's symbol in %s", v, sum)
	}
	sum.Amount += v.Amount
	return nil
}

type dayCreator struct {
	day     uint32
	creator uint64
}

type tally struct {
	network  map[uint32]*store.DailyFigures
	creators map[dayCreator]*store.DailyFigures
}

func tallyCreations(creations []store.Creation) (*tally, error) {
	t := &tally{
		network:  make(map[uint32]*store.DailyFigures),
		creators: make(map[dayCreator]*store.DailyFigures),
	}
	for i := range creations {
		c := &creations[i]
		day := store.DayFromBlockTime(c.BlockTime)
		n := t.network[day]
		if n == nil {
			n = &store.DailyFigures{}
			t.network[day] = n
		}
		dc := dayCreator{day, c.Creator}
		p := t.creators[dc]
		if p == nil {
			p = &store.DailyFigures{}
			t.creators[dc] = p
		}
		if err := Add(n, c); err != nil {
			return nil, err
		}
		if err := Add(p, c); err != nil {
			return nil, err
		}
	}
	return t, nil
}

func Apply(b *pebble.Batch, creations []store.Creation) error {
	if len(creations) == 0 {
		return nil
	}
	t, err := tallyCreations(creations)
	if err != nil {
		return err
	}
	for day, add := range t.network {
		if err := mergeInto(b, store.DailyNetworkKey(day), add); err != nil {
			return err
		}
	}
	for dc, add := range t.creators {
		d, err := merged(b, store.DailyCreatorDayKey(dc.creator, dc.day), add)
		if err != nil {
			return err
		}
		if err := store.SetDailyFigures(b, store.DailyCreatorDayKey(dc.creator, dc.day), d); err != nil {
			return err
		}
		if err := store.SetDailyFigures(b, store.DailyDayCreatorKey(dc.day, dc.creator), d); err != nil {
			return err
		}
	}
	return nil
}

func merged(b *pebble.Batch, k []byte, add *store.DailyFigures) (*store.DailyFigures, error) {
	d, err := store.GetDailyFigures(b, k)
	if err != nil {
		return nil, err
	}
	return d, Merge(d, add)
}

func mergeInto(b *pebble.Batch, k []byte, add *store.DailyFigures) error {
	d, err := merged(b, k, add)
	if err != nil {
		return err
	}
	return store.SetDailyFigures(b, k, d)
}
