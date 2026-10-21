package attribution

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/greymass/creationindex/internal/store"
	"github.com/greymass/roborovski/libraries/actionstream"
	"github.com/greymass/roborovski/libraries/chain"
)

type num int64

func (n *num) UnmarshalJSON(b []byte) error {
	if len(b) >= 2 && b[0] == '"' {
		b = b[1 : len(b)-1]
	}
	v, err := strconv.ParseInt(string(b), 10, 64)
	*n = num(v)
	return err
}

type newaccountData struct {
	Creator string `json:"creator"`
	Name    string `json:"name"`
}

type buyramData struct {
	Payer    string `json:"payer"`
	Receiver string `json:"receiver"`
	Quant    string `json:"quant"`
}

type buyrambytesData struct {
	Payer    string `json:"payer"`
	Receiver string `json:"receiver"`
	Bytes    num    `json:"bytes"`
}

type buyramselfData struct {
	Account string `json:"account"`
	Quant   string `json:"quant"`
}

type transferData struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Bytes num    `json:"bytes"`
	Memo  string `json:"memo"`
}

type setacctramData struct {
	Account  string `json:"account"`
	RAMBytes *num   `json:"ram_bytes"`
}

type logbuyramData struct {
	Quantity string `json:"quantity"`
	Bytes    num    `json:"bytes"`
	Fee      string `json:"fee"`
}

type contribution struct {
	to      uint64
	ordinal uint32
	c       store.Contribution
}

type buyLog struct {
	bytes    int64
	quantity *chain.Asset
	fee      *chain.Asset
}

// Attribute expects one transaction's actions in global seq order.
func Attribute(actions []actionstream.Action, programAccount uint64) ([]store.Creation, error) {
	var creations []store.Creation
	var contributions []contribution
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
			trx, err := trxID(a.TrxID)
			if err != nil {
				return nil, err
			}
			creations = append(creations, store.Creation{
				Account:   chain.StringToName(d.Name),
				Creator:   chain.StringToName(d.Creator),
				BlockNum:  a.BlockNum,
				BlockTime: a.BlockTime,
				TrxID:     trx,
				GlobalSeq: a.GlobalSeq,
			})
		case "logbuyram":
			var d logbuyramData
			if err := decode(a, &d); err != nil {
				return nil, err
			}
			q, err := asset(a, d.Quantity)
			if err != nil {
				return nil, err
			}
			fee, err := asset(a, d.Fee)
			if err != nil {
				return nil, err
			}
			if a.CreatorActionOrdinal != 0 {
				logs[a.CreatorActionOrdinal] = buyLog{bytes: int64(d.Bytes), quantity: q, fee: fee}
			}
		default:
			c, err := contributionOf(a, programAccount)
			if err != nil {
				return nil, err
			}
			if c != nil {
				contributions = append(contributions, *c)
			}
		}
	}

	if len(creations) == 0 {
		return nil, nil
	}
	byAccount := make(map[uint64]*store.Creation, len(creations))
	for i := range creations {
		byAccount[creations[i].Account] = &creations[i]
	}
	for i := range contributions {
		cr := byAccount[contributions[i].to]
		if cr == nil {
			continue
		}
		c := contributions[i].c
		if log, ok := logs[contributions[i].ordinal]; ok && c.Kind == store.KindBuy {
			c.Bytes = &log.bytes
			c.RequestedBytes = nil
			c.Quantity = log.quantity
			c.Fee = log.fee
		}
		cr.Contributions = append(cr.Contributions, c)
	}
	return creations, nil
}

func contributionOf(a *actionstream.Action, programAccount uint64) (*contribution, error) {
	c := contribution{ordinal: a.ActionOrdinal, c: store.Contribution{Action: chain.StringToName(a.Action), GlobalSeq: a.GlobalSeq}}
	switch a.Action {
	case "buyram":
		var d buyramData
		if err := decode(a, &d); err != nil {
			return nil, err
		}
		q, err := asset(a, d.Quant)
		if err != nil {
			return nil, err
		}
		c.to = chain.StringToName(d.Receiver)
		c.c.Kind = store.KindBuy
		c.c.Payer = chain.StringToName(d.Payer)
		c.c.Quantity = q
	case "buyrambytes":
		var d buyrambytesData
		if err := decode(a, &d); err != nil {
			return nil, err
		}
		c.to = chain.StringToName(d.Receiver)
		c.c.Kind = store.KindBuy
		c.c.Payer = chain.StringToName(d.Payer)
		requested := int64(d.Bytes)
		c.c.RequestedBytes = &requested
	case "buyramself":
		var d buyramselfData
		if err := decode(a, &d); err != nil {
			return nil, err
		}
		q, err := asset(a, d.Quant)
		if err != nil {
			return nil, err
		}
		c.to = chain.StringToName(d.Account)
		c.c.Kind = store.KindBuy
		c.c.Payer = c.to
		c.c.Quantity = q
	case "ramtransfer", "giftram":
		var d transferData
		if err := decode(a, &d); err != nil {
			return nil, err
		}
		bytes := int64(d.Bytes)
		c.to = chain.StringToName(d.To)
		c.c.Kind = store.KindTransfer
		c.c.Payer = chain.StringToName(d.From)
		c.c.Bytes = &bytes
		c.c.Memo = d.Memo
		if a.Action == "giftram" {
			c.c.Kind = store.KindGift
			if c.c.Payer == programAccount {
				c.c.Kind = store.KindProgramGift
			}
		}
	case "setacctram":
		var d setacctramData
		if err := decode(a, &d); err != nil {
			return nil, err
		}
		c.to = chain.StringToName(d.Account)
		c.c.Kind = store.KindQuotaSet
		if d.RAMBytes != nil {
			bytes := int64(*d.RAMBytes)
			c.c.Bytes = &bytes
		}
	default:
		return nil, nil
	}
	return &c, nil
}

// decode reads binary data when the stream could not decode it with an ABI.
func decode(a *actionstream.Action, v unpacker) error {
	if !json.Valid(a.ActionData) {
		r := binReader{b: a.ActionData}
		v.unpack(&r)
		if r.err != nil {
			return fmt.Errorf("attribution: decode %s seq %d: %w", a.Action, a.GlobalSeq, r.err)
		}
		return nil
	}
	if err := json.Unmarshal(a.ActionData, v); err != nil {
		return fmt.Errorf("attribution: decode %s seq %d: %w", a.Action, a.GlobalSeq, err)
	}
	return nil
}

func asset(a *actionstream.Action, s string) (*chain.Asset, error) {
	if s == "" {
		return nil, nil
	}
	v, err := chain.ParseAsset(s)
	if err != nil {
		return nil, fmt.Errorf("attribution: %s seq %d: asset %q: %w", a.Action, a.GlobalSeq, s, err)
	}
	return &v, nil
}

func trxID(s string) ([32]byte, error) {
	var id [32]byte
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != len(id) {
		return id, fmt.Errorf("attribution: trx_id %q is not 32 hex bytes", s)
	}
	copy(id[:], b)
	return id, nil
}
