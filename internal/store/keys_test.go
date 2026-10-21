package store

import (
	"bytes"
	"testing"
	"time"

	"github.com/greymass/roborovski/libraries/chain"
)

var (
	nameGm      = chain.StringToName("gm")
	nameMax     = ^uint64(0)
	daySamples  = []uint32{0, 1, 20381, ^uint32(0)}
	nameSamples = []uint64{0, 1, nameGm, chain.StringToName("aaaisn.b.sat"), nameMax}
	seqSamples  = []uint64{0, 1, 255, 256, 368360304290, ^uint64(0)}
)

func TestOneNameKeys(t *testing.T) {
	cases := []struct {
		enc func(uint64) []byte
		dec func([]byte) (uint64, bool)
	}{
		{CreationKey, DecodeCreationKey},
		{ProgramEventKey, DecodeProgramEventKey},
		{GiftAcctKey, DecodeGiftAcctKey},
		{GiftedBalanceKey, DecodeGiftedBalanceKey},
	}
	for i, c := range cases {
		for _, v := range append(nameSamples, seqSamples...) {
			got, ok := c.dec(c.enc(v))
			if !ok || got != v {
				t.Errorf("case %d: round trip %d = %d, %v", i, v, got, ok)
			}
		}
	}
}

func TestTwoPartKeys(t *testing.T) {
	cases := []struct {
		enc func(a, b uint64) []byte
		dec func([]byte) (uint64, uint64, bool)
	}{
		{CreatorIndexKey, DecodeCreatorIndexKey},
		{ProgramCreatorIndexKey, DecodeProgramCreatorIndexKey},
	}
	for i, c := range cases {
		for _, a := range nameSamples {
			for _, b := range seqSamples {
				ga, gb, ok := c.dec(c.enc(a, b))
				if !ok || ga != a || gb != b {
					t.Errorf("case %d: round trip (%d, %d) = (%d, %d), %v", i, a, b, ga, gb, ok)
				}
			}
		}
	}
}

func TestDayKeys(t *testing.T) {
	for _, d := range daySamples {
		if got, ok := DecodeDailyNetworkKey(DailyNetworkKey(d)); !ok || got != d {
			t.Errorf("network day %d = %d, %v", d, got, ok)
		}
		for _, n := range nameSamples {
			if gn, gd, ok := DecodeDailyCreatorDayKey(DailyCreatorDayKey(n, d)); !ok || gn != n || gd != d {
				t.Errorf("creator-day (%d, %d) = (%d, %d), %v", n, d, gn, gd, ok)
			}
			if gd, gn, ok := DecodeDailyDayCreatorKey(DailyDayCreatorKey(d, n)); !ok || gn != n || gd != d {
				t.Errorf("day-creator (%d, %d) = (%d, %d), %v", d, n, gd, gn, ok)
			}
		}
		for _, s := range seqSamples {
			if gd, gs, ok := DecodeProgramGiftDayKey(ProgramGiftDayKey(d, s)); !ok || gd != d || gs != s {
				t.Errorf("program gift day (%d, %d) = (%d, %d), %v", d, s, gd, gs, ok)
			}
		}
	}
}

func TestKeysSortNumerically(t *testing.T) {
	ordered := [][][]byte{
		{CreatorIndexKey(nameGm, 255), CreatorIndexKey(nameGm, 256), CreatorIndexKey(nameGm+1, 0)},
		{DailyCreatorDayKey(nameGm, 255), DailyCreatorDayKey(nameGm, 256), DailyCreatorDayKey(nameGm+1, 0)},
		{DailyDayCreatorKey(255, nameMax), DailyDayCreatorKey(256, 0)},
		{ProgramGiftDayKey(255, nameMax), ProgramGiftDayKey(256, 0)},
		{ProgramEventKey(255), ProgramEventKey(256)},
	}
	for i, keys := range ordered {
		for j := 1; j < len(keys); j++ {
			if bytes.Compare(keys[j-1], keys[j]) >= 0 {
				t.Errorf("set %d: key %d does not sort before key %d", i, j-1, j)
			}
		}
	}
}

func TestPrefixesBoundTheirKeys(t *testing.T) {
	cases := []struct {
		prefix []byte
		inside [][]byte
	}{
		{CreatorIndexPrefix(nameGm), [][]byte{CreatorIndexKey(nameGm, 0), CreatorIndexKey(nameGm, ^uint64(0))}},
		{DailyCreatorDayPrefix(nameGm), [][]byte{DailyCreatorDayKey(nameGm, 0), DailyCreatorDayKey(nameGm, ^uint32(0))}},
		{DailyDayCreatorPrefix(7), [][]byte{DailyDayCreatorKey(7, 0), DailyDayCreatorKey(7, nameMax)}},
		{ProgramGiftDayPrefix(7), [][]byte{ProgramGiftDayKey(7, 0), ProgramGiftDayKey(7, ^uint64(0))}},
		{ProgramCreatorIndexPrefix(nameGm), [][]byte{ProgramCreatorIndexKey(nameGm, 0), ProgramCreatorIndexKey(nameGm, ^uint64(0))}},
		{ProgramEventPrefix(), [][]byte{ProgramEventKey(0), ProgramEventKey(^uint64(0))}},
	}
	for i, c := range cases {
		end := PrefixEnd(c.prefix)
		for _, k := range c.inside {
			if bytes.Compare(k, c.prefix) < 0 || bytes.Compare(k, end) >= 0 {
				t.Errorf("case %d: key %x outside [%x, %x)", i, k, c.prefix, end)
			}
		}
	}
	if got := PrefixEnd([]byte{0x02, 0xff, 0xff}); !bytes.Equal(got, []byte{0x03}) {
		t.Errorf("PrefixEnd carry = %x", got)
	}
	if got := PrefixEnd([]byte{0xff}); got != nil {
		t.Errorf("PrefixEnd(ff) = %x, want nil", got)
	}
}

func TestDecodersRejectOtherKeys(t *testing.T) {
	if _, ok := DecodeCreationKey(GiftAcctKey(nameGm)); ok {
		t.Error("creation decoder accepted a giftacct key")
	}
	if _, _, ok := DecodeCreatorIndexKey(CreatorIndexPrefix(nameGm)); ok {
		t.Error("creator index decoder accepted a prefix")
	}
	if _, _, ok := DecodeDailyDayCreatorKey(DailyCreatorDayKey(nameGm, 1)); ok {
		t.Error("day-creator decoder accepted a creator-day key")
	}
}

func TestKeyspacesDistinct(t *testing.T) {
	keys := [][]byte{
		schemaKey(), cursorKey(StreamEosio), cursorKey(StreamGift), creationCountKey(),
		CreationKey(1), CreatorIndexKey(1, 1), DailyNetworkKey(1), DailyCreatorDayKey(1, 1),
		DailyDayCreatorKey(1, 1), ProgramEventKey(1), ProgramGiftDayKey(1, 1), GiftAcctKey(1),
		ProgramCreatorIndexKey(1, 1), GiftedBalanceKey(1),
	}
	seen := map[string]bool{}
	for _, k := range keys {
		head := string(k[:1])
		if k[0] == prefixMeta {
			head = string(k)
		}
		if seen[head] {
			t.Errorf("keyspace %x reused", head)
		}
		seen[head] = true
	}
}

func TestDayFromBlockTime(t *testing.T) {
	cases := []string{"2000-01-01T00:00:00Z", "2018-06-14T23:59:59.5Z", "2026-10-20T22:29:07Z", "2026-10-21T00:00:00Z"}
	for _, s := range cases {
		ts, err := time.Parse(time.RFC3339Nano, s)
		if err != nil {
			t.Fatal(err)
		}
		blockTime := uint32(ts.UnixMilli()/500 - blockTimeEpoch*2)
		want := uint32(ts.Unix() / 86400)
		if got := DayFromBlockTime(blockTime); got != want {
			t.Errorf("DayFromBlockTime(%s) = %d, want %d", s, got, want)
		}
	}
}
