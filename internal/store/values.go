package store

import (
	"encoding/binary"
	"fmt"

	"github.com/greymass/roborovski/libraries/chain"
)

const (
	versionCreation       byte = 1
	versionDailyFigures   byte = 1
	versionProgramEvent   byte = 1
	versionGiftAcct       byte = 1
	versionGiftedBalance  byte = 1
	versionCursor         byte = 1
	versionProgramCreator byte = 1
)

type Kind uint8

const (
	KindBuy Kind = iota + 1
	KindTransfer
	KindGift
	KindProgramGift
	KindQuotaSet
)

const kindCount = 5

var kindNames = [kindCount + 1]string{"", "buy", "transfer", "gift", "program_gift", "quota_set"}

func (k Kind) Valid() bool { return k >= KindBuy && k <= KindQuotaSet }

func (k Kind) String() string {
	if !k.Valid() {
		return fmt.Sprintf("kind(%d)", uint8(k))
	}
	return kindNames[k]
}

type Creation struct {
	Account       uint64
	Creator       uint64
	BlockNum      uint32
	BlockTime     uint32
	TrxID         [32]byte
	GlobalSeq     uint64
	Contributions []Contribution
}

type Contribution struct {
	Kind           Kind
	Action         uint64
	GlobalSeq      uint64
	Payer          uint64
	Bytes          *int64
	RequestedBytes *int64
	Quantity       *chain.Asset
	Fee            *chain.Asset
	Memo           string
}

const (
	hasBytes uint8 = 1 << iota
	hasRequestedBytes
	hasQuantity
	hasFee
	hasDailyQuota
	inCreation
)

func EncodeCreation(c *Creation) []byte {
	w := newWriter(versionCreation)
	w.u64(c.Account)
	w.u64(c.Creator)
	w.u32(c.BlockNum)
	w.u32(c.BlockTime)
	w.trx(c.TrxID)
	w.u64(c.GlobalSeq)
	w.uvarint(uint64(len(c.Contributions)))
	for i := range c.Contributions {
		writeContribution(w, &c.Contributions[i])
	}
	return w.b
}

func DecodeCreation(b []byte) (*Creation, error) {
	r := newReader(b, versionCreation, "creation")
	c := &Creation{
		Account:   r.u64(),
		Creator:   r.u64(),
		BlockNum:  r.u32(),
		BlockTime: r.u32(),
		TrxID:     r.trx(),
		GlobalSeq: r.u64(),
	}
	n := r.uvarint()
	if n > uint64(len(r.b)) {
		r.fail("contributions")
	}
	if r.err == nil && n > 0 {
		c.Contributions = make([]Contribution, n)
		for i := range c.Contributions {
			readContribution(r, &c.Contributions[i])
		}
	}
	if err := r.done(); err != nil {
		return nil, err
	}
	return c, nil
}

func writeContribution(w *writer, c *Contribution) {
	w.u8(uint8(c.Kind))
	w.u64(c.Action)
	w.u64(c.GlobalSeq)
	w.u64(c.Payer)
	var flags uint8
	if c.Bytes != nil {
		flags |= hasBytes
	}
	if c.RequestedBytes != nil {
		flags |= hasRequestedBytes
	}
	if c.Quantity != nil {
		flags |= hasQuantity
	}
	if c.Fee != nil {
		flags |= hasFee
	}
	w.u8(flags)
	if c.Bytes != nil {
		w.varint(*c.Bytes)
	}
	if c.RequestedBytes != nil {
		w.varint(*c.RequestedBytes)
	}
	if c.Quantity != nil {
		w.asset(*c.Quantity)
	}
	if c.Fee != nil {
		w.asset(*c.Fee)
	}
	w.str(c.Memo)
}

func readContribution(r *reader, c *Contribution) {
	c.Kind = Kind(r.u8())
	if r.err == nil && !c.Kind.Valid() {
		r.err = fmt.Errorf("%w: contribution kind %d", ErrCorrupt, c.Kind)
	}
	c.Action = r.u64()
	c.GlobalSeq = r.u64()
	c.Payer = r.u64()
	flags := r.u8()
	if flags&hasBytes != 0 {
		v := r.varint()
		c.Bytes = &v
	}
	if flags&hasRequestedBytes != 0 {
		v := r.varint()
		c.RequestedBytes = &v
	}
	if flags&hasQuantity != 0 {
		v := r.asset()
		c.Quantity = &v
	}
	if flags&hasFee != 0 {
		v := r.asset()
		c.Fee = &v
	}
	c.Memo = r.str()
}

type Figures struct {
	Contributions  uint64
	Bytes          uint64
	WithoutBytes   uint64
	RequestedBytes uint64
	Quantity       chain.Asset
}

type DailyFigures struct {
	Creations           uint64
	CreationsWithoutRAM uint64
	Kinds               [kindCount]Figures
}

func (d *DailyFigures) Kind(k Kind) *Figures { return &d.Kinds[k-1] }

func EncodeDailyFigures(d *DailyFigures) []byte {
	w := newWriter(versionDailyFigures)
	w.uvarint(d.Creations)
	w.uvarint(d.CreationsWithoutRAM)
	for i := range d.Kinds {
		f := &d.Kinds[i]
		w.uvarint(f.Contributions)
		w.uvarint(f.Bytes)
		w.uvarint(f.WithoutBytes)
		w.uvarint(f.RequestedBytes)
		w.asset(f.Quantity)
	}
	return w.b
}

func DecodeDailyFigures(b []byte) (*DailyFigures, error) {
	r := newReader(b, versionDailyFigures, "daily figures")
	d := &DailyFigures{Creations: r.uvarint(), CreationsWithoutRAM: r.uvarint()}
	for i := range d.Kinds {
		d.Kinds[i] = Figures{
			Contributions:  r.uvarint(),
			Bytes:          r.uvarint(),
			WithoutBytes:   r.uvarint(),
			RequestedBytes: r.uvarint(),
			Quantity:       r.asset(),
		}
	}
	if err := r.done(); err != nil {
		return nil, err
	}
	return d, nil
}

type EventType uint8

const (
	EventAdmin EventType = iota + 1
	EventProgramGift
	EventReturn
	EventRAMMovement
)

var eventTypeNames = [...]string{"", "admin", "program_gift", "return", "ram_movement"}

func (t EventType) Valid() bool { return t >= EventAdmin && t <= EventRAMMovement }

func (t EventType) String() string {
	if !t.Valid() {
		return fmt.Sprintf("event(%d)", uint8(t))
	}
	return eventTypeNames[t]
}

type ProgramEvent struct {
	Type            EventType
	Action          uint64
	GlobalSeq       uint64
	BlockNum        uint32
	BlockTime       uint32
	TrxID           [32]byte
	Creator         uint64
	From            uint64
	To              uint64
	Payer           uint64
	Bytes           *int64
	DailyQuotaBytes *int64
	Quantity        *chain.Asset
	Fee             *chain.Asset
	Memo            string
	InCreation      bool
}

func EncodeProgramEvent(e *ProgramEvent) []byte {
	w := newWriter(versionProgramEvent)
	w.u8(uint8(e.Type))
	w.u64(e.Action)
	w.u64(e.GlobalSeq)
	w.u32(e.BlockNum)
	w.u32(e.BlockTime)
	w.trx(e.TrxID)
	w.u64(e.Creator)
	w.u64(e.From)
	w.u64(e.To)
	w.u64(e.Payer)
	var flags uint8
	if e.Bytes != nil {
		flags |= hasBytes
	}
	if e.DailyQuotaBytes != nil {
		flags |= hasDailyQuota
	}
	if e.Quantity != nil {
		flags |= hasQuantity
	}
	if e.Fee != nil {
		flags |= hasFee
	}
	if e.InCreation {
		flags |= inCreation
	}
	w.u8(flags)
	if e.Bytes != nil {
		w.varint(*e.Bytes)
	}
	if e.DailyQuotaBytes != nil {
		w.varint(*e.DailyQuotaBytes)
	}
	if e.Quantity != nil {
		w.asset(*e.Quantity)
	}
	if e.Fee != nil {
		w.asset(*e.Fee)
	}
	w.str(e.Memo)
	return w.b
}

func DecodeProgramEvent(b []byte) (*ProgramEvent, error) {
	r := newReader(b, versionProgramEvent, "program event")
	e := &ProgramEvent{Type: EventType(r.u8())}
	if r.err == nil && !e.Type.Valid() {
		r.err = fmt.Errorf("%w: program event type %d", ErrCorrupt, e.Type)
	}
	e.Action = r.u64()
	e.GlobalSeq = r.u64()
	e.BlockNum = r.u32()
	e.BlockTime = r.u32()
	e.TrxID = r.trx()
	e.Creator = r.u64()
	e.From = r.u64()
	e.To = r.u64()
	e.Payer = r.u64()
	flags := r.u8()
	if flags&hasBytes != 0 {
		v := r.varint()
		e.Bytes = &v
	}
	if flags&hasDailyQuota != 0 {
		v := r.varint()
		e.DailyQuotaBytes = &v
	}
	if flags&hasQuantity != 0 {
		v := r.asset()
		e.Quantity = &v
	}
	if flags&hasFee != 0 {
		v := r.asset()
		e.Fee = &v
	}
	e.InCreation = flags&inCreation != 0
	e.Memo = r.str()
	if err := r.done(); err != nil {
		return nil, err
	}
	return e, nil
}

type GiftAcct struct {
	ProgramCreator uint64
	GlobalSeq      uint64
	BlockTime      uint32
	Bytes          int64
	Memo           string
}

func EncodeGiftAcct(g *GiftAcct) []byte {
	w := newWriter(versionGiftAcct)
	w.u64(g.ProgramCreator)
	w.u64(g.GlobalSeq)
	w.u32(g.BlockTime)
	w.varint(g.Bytes)
	w.str(g.Memo)
	return w.b
}

func DecodeGiftAcct(b []byte) (*GiftAcct, error) {
	r := newReader(b, versionGiftAcct, "giftacct")
	g := &GiftAcct{
		ProgramCreator: r.u64(),
		GlobalSeq:      r.u64(),
		BlockTime:      r.u32(),
		Bytes:          r.varint(),
		Memo:           r.str(),
	}
	if err := r.done(); err != nil {
		return nil, err
	}
	return g, nil
}

func EncodeGiftedBalance(bytes int64) []byte {
	w := newWriter(versionGiftedBalance)
	w.varint(bytes)
	return w.b
}

func DecodeGiftedBalance(b []byte) (int64, error) {
	r := newReader(b, versionGiftedBalance, "program-gifted balance")
	v := r.varint()
	return v, r.done()
}

type EntryType uint8

const (
	EntryGiftAcct EntryType = iota + 1
	EntryAdmin
)

type ProgramCreatorEntry struct {
	Type    EntryType
	Account uint64
}

func EncodeProgramCreatorEntry(e ProgramCreatorEntry) []byte {
	w := newWriter(versionProgramCreator)
	w.u8(uint8(e.Type))
	w.u64(e.Account)
	return w.b
}

func DecodeProgramCreatorEntry(b []byte) (ProgramCreatorEntry, error) {
	r := newReader(b, versionProgramCreator, "program creator entry")
	e := ProgramCreatorEntry{Type: EntryType(r.u8())}
	if r.err == nil && e.Type != EntryGiftAcct && e.Type != EntryAdmin {
		r.err = fmt.Errorf("%w: program creator entry type %d", ErrCorrupt, e.Type)
	}
	e.Account = r.u64()
	if err := r.done(); err != nil {
		return ProgramCreatorEntry{}, err
	}
	return e, nil
}

func EncodeName(name uint64) []byte { return binary.BigEndian.AppendUint64(nil, name) }

func DecodeName(b []byte) (uint64, error) {
	if len(b) != 8 {
		return 0, fmt.Errorf("%w: name length %d", ErrCorrupt, len(b))
	}
	return binary.BigEndian.Uint64(b), nil
}

type Stream uint8

const (
	StreamEosio Stream = iota + 1
	StreamGift
)

type Cursor struct {
	GlobalSeq uint64
	BlockTime uint32
}

func EncodeCursor(c Cursor) []byte {
	w := newWriter(versionCursor)
	w.u64(c.GlobalSeq)
	w.u32(c.BlockTime)
	return w.b
}

func DecodeCursor(b []byte) (Cursor, error) {
	r := newReader(b, versionCursor, "cursor")
	c := Cursor{GlobalSeq: r.u64(), BlockTime: r.u32()}
	if err := r.done(); err != nil {
		return Cursor{}, err
	}
	return c, nil
}
