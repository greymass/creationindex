package store

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/cockroachdb/pebble"
)

const SchemaVersion uint32 = 1

var ErrSchemaMismatch = errors.New("store: schema mismatch")

type Store struct {
	db *pebble.DB
}

func Open(path string) (*Store, error) {
	return open(path, &pebble.Options{})
}

func open(path string, opts *pebble.Options) (*Store, error) {
	db, err := pebble.Open(path, opts)
	if err != nil {
		return nil, fmt.Errorf("store: open %s: %w", path, err)
	}
	if err := checkSchema(db, path); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func checkSchema(db *pebble.DB, path string) error {
	found, err := readSchema(db)
	if err != nil {
		return err
	}
	if found == SchemaVersion {
		return nil
	}
	if found == 0 {
		empty, err := isEmpty(db)
		if err != nil {
			return err
		}
		if empty {
			return db.Set(schemaKey(), binary.BigEndian.AppendUint32(nil, SchemaVersion), pebble.Sync)
		}
	}
	return fmt.Errorf("%w: %s holds schema version %d, this build writes version %d; stop creationindex, delete %s, and start it again to replay from seq 1",
		ErrSchemaMismatch, path, found, SchemaVersion, path)
}

func readSchema(db *pebble.DB) (uint32, error) {
	val, closer, err := db.Get(schemaKey())
	if errors.Is(err, pebble.ErrNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	defer closer.Close()
	if len(val) != 4 {
		return 0, fmt.Errorf("%w: schema version length %d", ErrCorrupt, len(val))
	}
	return binary.BigEndian.Uint32(val), nil
}

func isEmpty(db *pebble.DB) (bool, error) {
	iter, err := db.NewIter(nil)
	if err != nil {
		return false, err
	}
	defer iter.Close()
	return !iter.First(), iter.Error()
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) DB() *pebble.DB { return s.db }

func (s *Store) NewBatch() *pebble.Batch { return s.db.NewIndexedBatch() }

func (s *Store) Commit(b *pebble.Batch) error {
	defer b.Close()
	return b.Commit(pebble.Sync)
}

func get(r pebble.Reader, k []byte) ([]byte, error) {
	val, closer, err := r.Get(k)
	if errors.Is(err, pebble.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer closer.Close()
	return append([]byte(nil), val...), nil
}

func GetCursor(r pebble.Reader, s Stream) (Cursor, error) {
	val, err := get(r, cursorKey(s))
	if err != nil || val == nil {
		return Cursor{}, err
	}
	return DecodeCursor(val)
}

func SetCursor(b *pebble.Batch, s Stream, c Cursor) error {
	return b.Set(cursorKey(s), EncodeCursor(c), nil)
}

func GetCreationCount(r pebble.Reader) (uint64, error) {
	val, err := get(r, creationCountKey())
	if err != nil || val == nil {
		return 0, err
	}
	if len(val) != 8 {
		return 0, fmt.Errorf("%w: creation count length %d", ErrCorrupt, len(val))
	}
	return binary.BigEndian.Uint64(val), nil
}

func SetCreationCount(b *pebble.Batch, n uint64) error {
	return b.Set(creationCountKey(), binary.BigEndian.AppendUint64(nil, n), nil)
}

func GetCreation(r pebble.Reader, account uint64) (*Creation, error) {
	val, err := get(r, CreationKey(account))
	if err != nil || val == nil {
		return nil, err
	}
	return DecodeCreation(val)
}

func PutCreation(b *pebble.Batch, c *Creation) error {
	if err := b.Set(CreationKey(c.Account), EncodeCreation(c), nil); err != nil {
		return err
	}
	return b.Set(CreatorIndexKey(c.Creator, c.GlobalSeq), EncodeName(c.Account), nil)
}

func GetDailyFigures(r pebble.Reader, k []byte) (*DailyFigures, error) {
	val, err := get(r, k)
	if err != nil {
		return nil, err
	}
	if val == nil {
		return &DailyFigures{}, nil
	}
	return DecodeDailyFigures(val)
}

func SetDailyFigures(b *pebble.Batch, k []byte, d *DailyFigures) error {
	return b.Set(k, EncodeDailyFigures(d), nil)
}
