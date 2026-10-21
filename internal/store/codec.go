package store

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/greymass/roborovski/libraries/chain"
)

var ErrCorrupt = errors.New("store: corrupt value")

type writer struct{ b []byte }

func newWriter(version byte) *writer { return &writer{b: []byte{version}} }

func (w *writer) u8(v uint8)       { w.b = append(w.b, v) }
func (w *writer) u32(v uint32)     { w.b = binary.BigEndian.AppendUint32(w.b, v) }
func (w *writer) u64(v uint64)     { w.b = binary.BigEndian.AppendUint64(w.b, v) }
func (w *writer) uvarint(v uint64) { w.b = binary.AppendUvarint(w.b, v) }
func (w *writer) varint(v int64)   { w.b = binary.AppendVarint(w.b, v) }
func (w *writer) trx(v [32]byte)   { w.b = append(w.b, v[:]...) }

func (w *writer) str(v string) {
	w.uvarint(uint64(len(v)))
	w.b = append(w.b, v...)
}

func (w *writer) asset(v chain.Asset) {
	w.varint(v.Amount)
	w.u64(v.Symbol)
}

type reader struct {
	b   []byte
	err error
}

func newReader(b []byte, version byte, what string) *reader {
	r := &reader{b: b}
	if len(b) == 0 {
		r.err = fmt.Errorf("%w: empty %s", ErrCorrupt, what)
	} else if b[0] != version {
		r.err = fmt.Errorf("%w: %s version %d, expected %d", ErrCorrupt, what, b[0], version)
	} else {
		r.b = b[1:]
	}
	return r
}

func (r *reader) fail(what string) {
	if r.err == nil {
		r.err = fmt.Errorf("%w: short %s", ErrCorrupt, what)
	}
}

func (r *reader) take(n int, what string) []byte {
	if r.err != nil || len(r.b) < n {
		r.fail(what)
		return nil
	}
	out := r.b[:n]
	r.b = r.b[n:]
	return out
}

func (r *reader) u8() uint8 {
	if b := r.take(1, "u8"); b != nil {
		return b[0]
	}
	return 0
}

func (r *reader) u32() uint32 {
	if b := r.take(4, "u32"); b != nil {
		return binary.BigEndian.Uint32(b)
	}
	return 0
}

func (r *reader) u64() uint64 {
	if b := r.take(8, "u64"); b != nil {
		return binary.BigEndian.Uint64(b)
	}
	return 0
}

func (r *reader) uvarint() uint64 {
	if r.err != nil {
		return 0
	}
	v, n := binary.Uvarint(r.b)
	if n <= 0 {
		r.fail("uvarint")
		return 0
	}
	r.b = r.b[n:]
	return v
}

func (r *reader) varint() int64 {
	if r.err != nil {
		return 0
	}
	v, n := binary.Varint(r.b)
	if n <= 0 {
		r.fail("varint")
		return 0
	}
	r.b = r.b[n:]
	return v
}

func (r *reader) trx() (v [32]byte) {
	if b := r.take(32, "trx_id"); b != nil {
		copy(v[:], b)
	}
	return v
}

func (r *reader) str() string {
	n := r.uvarint()
	if n > uint64(len(r.b)) {
		r.fail("string")
		return ""
	}
	return string(r.take(int(n), "string"))
}

func (r *reader) asset() chain.Asset {
	return chain.Asset{Amount: r.varint(), Symbol: r.u64()}
}

func (r *reader) done() error {
	if r.err == nil && len(r.b) != 0 {
		r.err = fmt.Errorf("%w: %d trailing bytes", ErrCorrupt, len(r.b))
	}
	return r.err
}
