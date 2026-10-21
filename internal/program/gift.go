package program

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/cockroachdb/pebble"
	"github.com/greymass/creationindex/internal/store"
	"github.com/greymass/roborovski/libraries/actionstream"
	"github.com/greymass/roborovski/libraries/chain"
)

var (
	actGiftAcct   = chain.StringToName("giftacct")
	actAddCreator = chain.StringToName("addcreator")
	actSetQuota   = chain.StringToName("setquota")
	actRmCreator  = chain.StringToName("rmcreator")
	actSuspend    = chain.StringToName("suspend")
	actResume     = chain.StringToName("resume")
	actEnable     = chain.StringToName("enable")
	actDisable    = chain.StringToName("disable")
)

var giftActions = []uint64{actGiftAcct, actAddCreator, actSetQuota, actRmCreator, actSuspend, actResume, actEnable, actDisable}

var ErrUndecodable = errors.New("program: undecodable action data")

type Outcome uint8

const (
	Ignored Outcome = iota
	Legacy
	Written
)

func GiftFilter(programAccount uint64) actionstream.Filter {
	return actionstream.Filter{
		Contracts: []uint64{programAccount},
		Actions:   append([]uint64(nil), giftActions...),
	}
}

type GiftStream struct {
	program string
}

func NewGiftStream(programAccount string) *GiftStream {
	return &GiftStream{program: programAccount}
}

type quota int64

func (q *quota) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		b = []byte(s)
	}
	v, err := strconv.ParseInt(string(b), 10, 64)
	*q = quota(v)
	return err
}

type giftAcctData struct {
	Creator *string `json:"creator"`
	Account *string `json:"account"`
	Bytes   quota   `json:"bytes"`
	Memo    string  `json:"memo"`
}

type adminData struct {
	Creator         *string `json:"creator"`
	DailyQuotaBytes *quota  `json:"daily_quota_bytes"`
}

func (g *GiftStream) Apply(b *pebble.Batch, a *actionstream.Action) (Outcome, error) {
	if a.Contract != g.program || a.Receiver != a.Contract {
		return Ignored, nil
	}
	act := chain.StringToName(a.Action)
	switch act {
	case actGiftAcct:
		return g.giftAcct(b, a)
	case actAddCreator, actSetQuota, actRmCreator, actSuspend, actResume, actEnable, actDisable:
		return g.admin(b, a, act)
	}
	return Ignored, nil
}

func (g *GiftStream) giftAcct(b *pebble.Batch, a *actionstream.Action) (Outcome, error) {
	var d giftAcctData
	if err := json.Unmarshal(a.ActionData, &d); err != nil {
		return Ignored, fmt.Errorf("%w: giftacct seq %d: %v", ErrUndecodable, a.GlobalSeq, err)
	}
	// The {op, to, bytes, memo} giftacct from before the a9a9a59 redeploy names neither field.
	if d.Creator == nil || d.Account == nil {
		return Legacy, nil
	}
	creator := chain.StringToName(*d.Creator)
	account := chain.StringToName(*d.Account)
	row := store.EncodeGiftAcct(&store.GiftAcct{
		ProgramCreator: creator,
		GlobalSeq:      a.GlobalSeq,
		BlockTime:      a.BlockTime,
		Bytes:          int64(d.Bytes),
		Memo:           d.Memo,
	})
	if err := b.Set(store.GiftAcctKey(account), row, nil); err != nil {
		return Ignored, err
	}
	entry := store.EncodeProgramCreatorEntry(store.ProgramCreatorEntry{Type: store.EntryGiftAcct, Account: account})
	if err := b.Set(store.ProgramCreatorIndexKey(creator, a.GlobalSeq), entry, nil); err != nil {
		return Ignored, err
	}
	return Written, nil
}

func (g *GiftStream) admin(b *pebble.Batch, a *actionstream.Action, act uint64) (Outcome, error) {
	trxID, err := parseTrxID(a.TrxID)
	if err != nil {
		return Ignored, fmt.Errorf("%w: %s seq %d: %v", ErrUndecodable, a.Action, a.GlobalSeq, err)
	}
	ev := &store.ProgramEvent{
		Type:      store.EventAdmin,
		Action:    act,
		GlobalSeq: a.GlobalSeq,
		BlockNum:  a.BlockNum,
		BlockTime: a.BlockTime,
		TrxID:     trxID,
	}
	if act != actEnable && act != actDisable {
		var d adminData
		if err := json.Unmarshal(a.ActionData, &d); err != nil {
			return Ignored, fmt.Errorf("%w: %s seq %d: %v", ErrUndecodable, a.Action, a.GlobalSeq, err)
		}
		if d.Creator == nil {
			return Ignored, fmt.Errorf("%w: %s seq %d: no creator", ErrUndecodable, a.Action, a.GlobalSeq)
		}
		ev.Creator = chain.StringToName(*d.Creator)
		if act == actAddCreator || act == actSetQuota {
			if d.DailyQuotaBytes == nil {
				return Ignored, fmt.Errorf("%w: %s seq %d: no daily_quota_bytes", ErrUndecodable, a.Action, a.GlobalSeq)
			}
			q := int64(*d.DailyQuotaBytes)
			ev.DailyQuotaBytes = &q
		}
	}
	if err := b.Set(store.ProgramEventKey(a.GlobalSeq), store.EncodeProgramEvent(ev), nil); err != nil {
		return Ignored, err
	}
	if ev.Creator != 0 {
		entry := store.EncodeProgramCreatorEntry(store.ProgramCreatorEntry{Type: store.EntryAdmin})
		if err := b.Set(store.ProgramCreatorIndexKey(ev.Creator, a.GlobalSeq), entry, nil); err != nil {
			return Ignored, err
		}
	}
	return Written, nil
}

func parseTrxID(s string) (id [32]byte, err error) {
	if hex.DecodedLen(len(s)) != len(id) {
		return id, fmt.Errorf("trx_id length %d", len(s))
	}
	_, err = hex.Decode(id[:], []byte(s))
	return id, err
}
