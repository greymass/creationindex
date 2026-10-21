package api

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/greymass/creationindex/internal/store"
	"github.com/greymass/roborovski/libraries/chain"
)

func TestQuotaWindowOpen(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "creationindex.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	pc, giftee := chain.StringToName("gm"), chain.StringToName("alice.gm")
	quota := int64(262144)
	const giftTime = 1600000000
	b := db.NewBatch()
	sets := map[string][]byte{
		string(store.ProgramEventKey(1)): store.EncodeProgramEvent(&store.ProgramEvent{
			Type: store.EventAdmin, Action: actAddCreator, GlobalSeq: 1, BlockTime: 2 * (giftTime - 600 - 946684800), Creator: pc, DailyQuotaBytes: &quota,
		}),
		string(store.ProgramCreatorIndexKey(pc, 1)): store.EncodeProgramCreatorEntry(store.ProgramCreatorEntry{Type: store.EntryAdmin}),
		string(store.GiftAcctKey(giftee)): store.EncodeGiftAcct(&store.GiftAcct{
			ProgramCreator: pc, GlobalSeq: 2, BlockTime: 2 * (giftTime - 946684800), Bytes: 4000,
		}),
		string(store.ProgramCreatorIndexKey(pc, 2)): store.EncodeProgramCreatorEntry(store.ProgramCreatorEntry{Type: store.EntryGiftAcct, Account: giftee}),
	}
	for k, v := range sets {
		if err := b.Set([]byte(k), v, nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Commit(b); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		after time.Duration
		open  bool
	}{{time.Hour, true}, {24*time.Hour - time.Second, true}, {24 * time.Hour, false}} {
		s := New(db, "gift.gm", "")
		s.now = func() time.Time { return time.Unix(giftTime, 0).Add(tc.after) }
		snap := db.DB().NewSnapshot()
		out, err := s.getQuota(snap, params{"program_creator": "gm"})
		snap.Close()
		if err != nil {
			t.Fatal(err)
		}
		w := out.(quotaHistoryJSON).Registrations[0].Windows[0]
		if w.Open != tc.open || w.UsedBytes != 4136 || w.Start != "2020-09-13T12:26:40.000Z" || w.End != "2020-09-14T12:26:40.000Z" {
			t.Errorf("%v after the gift: %+v, want open=%v", tc.after, *w, tc.open)
		}
	}
}

func TestTopUpAndWindowBoundary(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "creationindex.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	program, pc, giftee := chain.StringToName("gift.gm"), chain.StringToName("gm"), chain.StringToName("alice.gm")
	quota, bytes, topUp := int64(262144), int64(4000), int64(1024)
	const t0 = 2 * (1600000000 - 946684800)
	day := store.DayFromBlockTime(t0)
	gift := func(seq uint64, bt uint32, b *int64, inCreation bool) []byte {
		return store.EncodeProgramEvent(&store.ProgramEvent{
			Type: store.EventProgramGift, Action: chain.StringToName("giftram"), GlobalSeq: seq, BlockTime: bt, From: program, To: giftee, Bytes: b, InCreation: inCreation,
		})
	}
	sets := map[string][]byte{
		string(store.ProgramEventKey(1)): store.EncodeProgramEvent(&store.ProgramEvent{
			Type: store.EventAdmin, Action: actAddCreator, GlobalSeq: 1, BlockTime: t0 - 2, Creator: pc, DailyQuotaBytes: &quota,
		}),
		string(store.ProgramCreatorIndexKey(pc, 1)):             store.EncodeProgramCreatorEntry(store.ProgramCreatorEntry{Type: store.EntryAdmin}),
		string(store.GiftAcctKey(giftee)):                       store.EncodeGiftAcct(&store.GiftAcct{ProgramCreator: pc, GlobalSeq: 2, BlockTime: t0, Bytes: bytes}),
		string(store.ProgramCreatorIndexKey(pc, 2)):             store.EncodeProgramCreatorEntry(store.ProgramCreatorEntry{Type: store.EntryGiftAcct, Account: giftee}),
		string(store.ProgramEventKey(3)):                        gift(3, t0, &bytes, true),
		string(store.ProgramGiftDayKey(day, 3)):                 nil,
		string(store.ProgramEventKey(4)):                        gift(4, t0+2, &topUp, false),
		string(store.ProgramGiftDayKey(day, 4)):                 nil,
		string(store.GiftAcctKey(chain.StringToName("bob.gm"))): store.EncodeGiftAcct(&store.GiftAcct{ProgramCreator: pc, GlobalSeq: 5, BlockTime: t0 + 2*86400, Bytes: bytes}),
		string(store.ProgramCreatorIndexKey(pc, 5)):             store.EncodeProgramCreatorEntry(store.ProgramCreatorEntry{Type: store.EntryGiftAcct, Account: chain.StringToName("bob.gm")}),
	}
	b := db.NewBatch()
	for k, v := range sets {
		if err := b.Set([]byte(k), v, nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Commit(b); err != nil {
		t.Fatal(err)
	}
	s := New(db, "gift.gm", "")
	snap := db.DB().NewSnapshot()
	defer snap.Close()

	out, err := s.getProgramDaily(snap, params{"from": dayString(day), "to": dayString(day)})
	if err != nil {
		t.Fatal(err)
	}
	rows := out.(programDailyJSON).Days[0].ProgramCreators
	if len(rows) != 2 || rows[0].ProgramCreator != "" || rows[0].Bytes != topUp || rows[1].ProgramCreator != "gm" || rows[1].Bytes != bytes {
		t.Errorf("program daily rows %+v %+v; want the top-up apart from gm's gift", rows[0], rows[1])
	}

	ev, err := s.getProgramEvents(snap, params{"type": "program_gift"})
	if err != nil {
		t.Fatal(err)
	}
	if e := ev.(programEventsJSON).Events; len(e) != 2 || e[0].ProgramCreator != "gm" || e[1].ProgramCreator != "" {
		t.Errorf("program gift events %+v", e)
	}

	q, err := s.getQuota(snap, params{"program_creator": "gm"})
	if err != nil {
		t.Fatal(err)
	}
	if w := q.(quotaHistoryJSON).Registrations[0].Windows; len(w) != 2 || w[0].Gifts != 1 || w[1].Gifts != 1 {
		t.Errorf("a gift exactly %d s after the window opened must open a new one: %d windows", quotaWindowSeconds, len(w))
	}
}
