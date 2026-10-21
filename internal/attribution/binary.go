package attribution

import (
	"encoding/binary"
	"errors"

	"github.com/greymass/roborovski/libraries/chain"
)

var errShort = errors.New("short binary action data")

type unpacker interface {
	unpack(r *binReader)
}

type binReader struct {
	b   []byte
	err error
}

func (r *binReader) take(n int) []byte {
	if r.err != nil || len(r.b) < n {
		r.err = errShort
		return nil
	}
	out := r.b[:n]
	r.b = r.b[n:]
	return out
}

func (r *binReader) u64() uint64 {
	if b := r.take(8); b != nil {
		return binary.LittleEndian.Uint64(b)
	}
	return 0
}

func (r *binReader) u32() uint32 {
	if b := r.take(4); b != nil {
		return binary.LittleEndian.Uint32(b)
	}
	return 0
}

func (r *binReader) u8() uint8 {
	if b := r.take(1); b != nil {
		return b[0]
	}
	return 0
}

func (r *binReader) name() string { return chain.NameToString(r.u64()) }

func (r *binReader) asset() string {
	amount := int64(r.u64())
	return chain.Asset{Amount: amount, Symbol: r.u64()}.String()
}

func (r *binReader) str() string {
	n, size := binary.Uvarint(r.b)
	if r.err != nil || size <= 0 || n > uint64(len(r.b)-size) {
		r.err = errShort
		return ""
	}
	r.b = r.b[size:]
	return string(r.take(int(n)))
}

func (d *newaccountData) unpack(r *binReader) {
	d.Creator = r.name()
	d.Name = r.name()
}

func (d *buyramData) unpack(r *binReader) {
	d.Payer = r.name()
	d.Receiver = r.name()
	d.Quant = r.asset()
}

func (d *buyrambytesData) unpack(r *binReader) {
	d.Payer = r.name()
	d.Receiver = r.name()
	d.Bytes = num(r.u32())
}

func (d *buyramselfData) unpack(r *binReader) {
	d.Account = r.name()
	d.Quant = r.asset()
}

func (d *transferData) unpack(r *binReader) {
	d.From = r.name()
	d.To = r.name()
	d.Bytes = num(int64(r.u64()))
	d.Memo = r.str()
}

func (d *setacctramData) unpack(r *binReader) {
	d.Account = r.name()
	if r.u8() != 0 {
		v := num(int64(r.u64()))
		d.RAMBytes = &v
	}
}

func (d *logbuyramData) unpack(r *binReader) {
	r.name()
	r.name()
	d.Quantity = r.asset()
	d.Bytes = num(int64(r.u64()))
	r.u64()
	if len(r.b) > 0 {
		d.Fee = r.asset()
	}
}
