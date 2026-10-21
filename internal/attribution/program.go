package attribution

import (
	"github.com/greymass/creationindex/internal/store"
	"github.com/greymass/roborovski/libraries/actionstream"
	"github.com/greymass/roborovski/libraries/chain"
)

type ungiftramData struct {
	From string `json:"from"`
	To   string `json:"to"`
	Memo string `json:"memo"`
}

func (d *ungiftramData) unpack(r *binReader) {
	d.From = r.name()
	d.To = r.name()
	d.Memo = r.str()
}

// ProgramEvents expects one transaction's actions in global seq order; a return's bytes stay nil for the caller to fill from the balance.
func ProgramEvents(actions []actionstream.Action, programAccount uint64) ([]store.ProgramEvent, error) {
	if programAccount == 0 {
		return nil, nil
	}
	created := make(map[uint64]bool)
	logs := make(map[uint32]buyLog)
	for i := range actions {
		a := &actions[i]
		if a.Contract != System || a.Receiver != System {
			continue
		}
		switch a.Action {
		case "newaccount":
			var d newaccountData
			if err := decode(a, &d); err != nil {
				return nil, err
			}
			created[chain.StringToName(d.Name)] = true
		case "logbuyram":
			if err := readBuyLog(a, logs); err != nil {
				return nil, err
			}
		}
	}

	var events []store.ProgramEvent
	for i := range actions {
		a := &actions[i]
		if a.Contract != System || a.Receiver != System {
			continue
		}
		e, err := programEventOf(a, programAccount, logs)
		if err != nil {
			return nil, err
		}
		if e == nil {
			continue
		}
		trx, err := trxID(a.TrxID)
		if err != nil {
			return nil, err
		}
		e.Action = chain.StringToName(a.Action)
		e.GlobalSeq = a.GlobalSeq
		e.BlockNum = a.BlockNum
		e.BlockTime = a.BlockTime
		e.TrxID = trx
		e.InCreation = created[e.To]
		events = append(events, *e)
	}
	return events, nil
}

func programEventOf(a *actionstream.Action, programAccount uint64, logs map[uint32]buyLog) (*store.ProgramEvent, error) {
	e := store.ProgramEvent{Type: store.EventRAMMovement}
	switch a.Action {
	case "ungiftram":
		var d ungiftramData
		if err := decode(a, &d); err != nil {
			return nil, err
		}
		e.From = chain.StringToName(d.From)
		e.To = chain.StringToName(d.To)
		e.Memo = d.Memo
		if e.To == programAccount {
			e.Type = store.EventReturn
		}
	case "setacctram":
		var d setacctramData
		if err := decode(a, &d); err != nil {
			return nil, err
		}
		e.To = chain.StringToName(d.Account)
		if d.RAMBytes != nil {
			bytes := int64(*d.RAMBytes)
			e.Bytes = &bytes
		}
	default:
		c, err := contributionOf(a, programAccount)
		if err != nil || c == nil {
			return nil, err
		}
		e.To = c.to
		e.Bytes = c.c.Bytes
		e.Quantity = c.c.Quantity
		e.Memo = c.c.Memo
		switch c.c.Kind {
		case store.KindBuy:
			e.Payer = c.c.Payer
			if log, ok := logs[c.ordinal]; ok {
				e.Bytes = &log.bytes
				e.Quantity = log.quantity
				e.Fee = log.fee
			}
		case store.KindProgramGift:
			e.Type = store.EventProgramGift
			e.From = c.c.Payer
		default:
			e.From = c.c.Payer
		}
	}
	if e.Type == store.EventRAMMovement && e.From != programAccount && e.To != programAccount && e.Payer != programAccount {
		return nil, nil
	}
	return &e, nil
}
