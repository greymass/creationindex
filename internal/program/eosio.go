package program

import (
	"github.com/cockroachdb/pebble"
	"github.com/greymass/creationindex/internal/attribution"
	"github.com/greymass/creationindex/internal/store"
	"github.com/greymass/roborovski/libraries/actionstream"
)

// ApplyEosio writes one eosio transaction's program events, the program-gifted balances and the (day, seq) gift index.
func ApplyEosio(b *pebble.Batch, actions []actionstream.Action, programAccount uint64) error {
	events, err := attribution.ProgramEvents(actions, programAccount)
	if err != nil {
		return err
	}
	for i := range events {
		e := &events[i]
		switch e.Type {
		case store.EventProgramGift:
			balance, err := store.GetGiftedBalance(b, e.To)
			if err != nil {
				return err
			}
			if err := store.SetGiftedBalance(b, e.To, balance+*e.Bytes); err != nil {
				return err
			}
			if err := b.Set(store.ProgramGiftDayKey(store.DayFromBlockTime(e.BlockTime), e.GlobalSeq), nil, nil); err != nil {
				return err
			}
		case store.EventReturn:
			balance, err := store.GetGiftedBalance(b, e.From)
			if err != nil {
				return err
			}
			e.Bytes = &balance
			if err := store.SetGiftedBalance(b, e.From, 0); err != nil {
				return err
			}
		}
		if err := b.Set(store.ProgramEventKey(e.GlobalSeq), store.EncodeProgramEvent(e), nil); err != nil {
			return err
		}
	}
	return nil
}
