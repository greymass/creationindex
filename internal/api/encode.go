package api

import (
	"encoding/hex"
	"time"

	"github.com/cockroachdb/pebble"
	"github.com/greymass/creationindex/internal/store"
	"github.com/greymass/roborovski/libraries/chain"
)

const timeLayout = "2006-01-02T15:04:05.000Z"

func blockTime(bt uint32) string {
	return time.UnixMilli(store.BlockTimeUnixMilli(bt)).UTC().Format(timeLayout)
}

func dayString(day uint32) string {
	return time.Unix(int64(day)*86400, 0).UTC().Format(time.DateOnly)
}

func name(n uint64) string {
	if n == 0 {
		return ""
	}
	return chain.NameToString(n)
}

func assetString(a *chain.Asset) *string {
	if a == nil {
		return nil
	}
	s := a.String()
	return &s
}

type creationJSON struct {
	Account        string             `json:"account"`
	Creator        string             `json:"creator"`
	ProgramCreator string             `json:"program_creator"`
	BlockNum       uint32             `json:"block_num"`
	BlockTime      string             `json:"block_time"`
	TrxID          string             `json:"trx_id"`
	GlobalSeq      uint64             `json:"global_seq"`
	Contributions  []contributionJSON `json:"contributions"`
}

type contributionJSON struct {
	Kind           string  `json:"kind"`
	Action         string  `json:"action"`
	GlobalSeq      uint64  `json:"global_seq"`
	Payer          string  `json:"payer"`
	Bytes          *int64  `json:"bytes"`
	RequestedBytes *int64  `json:"requested_bytes"`
	Quantity       *string `json:"quantity"`
	Fee            *string `json:"fee"`
	Memo           string  `json:"memo"`
}

func encodeCreation(r pebble.Reader, c *store.Creation) (creationJSON, error) {
	g, err := store.GetGiftAcct(r, c.Account)
	if err != nil {
		return creationJSON{}, err
	}
	out := creationJSON{
		Account:       name(c.Account),
		Creator:       name(c.Creator),
		BlockNum:      c.BlockNum,
		BlockTime:     blockTime(c.BlockTime),
		TrxID:         hex.EncodeToString(c.TrxID[:]),
		GlobalSeq:     c.GlobalSeq,
		Contributions: make([]contributionJSON, len(c.Contributions)),
	}
	if g != nil {
		out.ProgramCreator = name(g.ProgramCreator)
	}
	for i := range c.Contributions {
		k := &c.Contributions[i]
		out.Contributions[i] = contributionJSON{
			Kind:           k.Kind.String(),
			Action:         name(k.Action),
			GlobalSeq:      k.GlobalSeq,
			Payer:          name(k.Payer),
			Bytes:          k.Bytes,
			RequestedBytes: k.RequestedBytes,
			Quantity:       assetString(k.Quantity),
			Fee:            assetString(k.Fee),
			Memo:           k.Memo,
		}
	}
	return out, nil
}

type figuresJSON struct {
	Contributions  uint64  `json:"contributions"`
	Bytes          *uint64 `json:"bytes"`
	WithoutBytes   uint64  `json:"without_bytes"`
	RequestedBytes *uint64 `json:"requested_bytes"`
	Quantity       *string `json:"quantity"`
}

type kindsJSON struct {
	Buy         figuresJSON `json:"buy"`
	Transfer    figuresJSON `json:"transfer"`
	Gift        figuresJSON `json:"gift"`
	ProgramGift figuresJSON `json:"program_gift"`
	QuotaSet    figuresJSON `json:"quota_set"`
}

// encodeFigures prints a sum as null when no contribution of the kind stated it.
func encodeFigures(f *store.Figures) figuresJSON {
	out := figuresJSON{Contributions: f.Contributions, WithoutBytes: f.WithoutBytes}
	if f.Contributions > f.WithoutBytes {
		v := f.Bytes
		out.Bytes = &v
	}
	if f.RequestedBytes > 0 {
		v := f.RequestedBytes
		out.RequestedBytes = &v
	}
	if f.Quantity.Symbol != 0 {
		out.Quantity = assetString(&f.Quantity)
	}
	return out
}

func encodeKinds(d *store.DailyFigures) kindsJSON {
	return kindsJSON{
		Buy:         encodeFigures(d.Kind(store.KindBuy)),
		Transfer:    encodeFigures(d.Kind(store.KindTransfer)),
		Gift:        encodeFigures(d.Kind(store.KindGift)),
		ProgramGift: encodeFigures(d.Kind(store.KindProgramGift)),
		QuotaSet:    encodeFigures(d.Kind(store.KindQuotaSet)),
	}
}
