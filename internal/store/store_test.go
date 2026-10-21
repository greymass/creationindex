package store

import (
	"encoding/binary"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cockroachdb/pebble"
)

func openTemp(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "creationindex.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return s, path
}

func rawSet(t *testing.T, path string, k, v []byte) {
	t.Helper()
	db, err := pebble.Open(path, &pebble.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Set(k, v, pebble.Sync); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestOpenWritesSchemaAndReopens(t *testing.T) {
	s, path := openTemp(t)
	got, err := readSchema(s.DB())
	if err != nil || got != SchemaVersion {
		t.Fatalf("schema = %d, %v", got, err)
	}
	s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	s.Close()
}

func TestSchemaMismatchRefusesStart(t *testing.T) {
	s, path := openTemp(t)
	s.Close()
	rawSet(t, path, schemaKey(), binary.BigEndian.AppendUint32(nil, SchemaVersion+1))
	_, err := Open(path)
	if !errors.Is(err, ErrSchemaMismatch) {
		t.Fatalf("err = %v, want ErrSchemaMismatch", err)
	}
	if !strings.Contains(err.Error(), "delete "+path) || !strings.Contains(err.Error(), "replay from seq 1") {
		t.Errorf("error does not state the wipe-and-replay step: %v", err)
	}
}

func TestUnversionedDataRefusesStart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	rawSet(t, path, CreationKey(1), []byte{1})
	if _, err := Open(path); !errors.Is(err, ErrSchemaMismatch) {
		t.Fatalf("err = %v, want ErrSchemaMismatch", err)
	}
}

func TestCursorsAreSeparateAndBatched(t *testing.T) {
	s, _ := openTemp(t)
	defer s.Close()

	for _, stream := range []Stream{StreamEosio, StreamGift} {
		c, err := GetCursor(s.DB(), stream)
		if err != nil || c != (Cursor{}) {
			t.Fatalf("fresh cursor %d = %+v, %v", stream, c, err)
		}
	}

	b := s.NewBatch()
	want := Cursor{GlobalSeq: 368360752482, BlockTime: 1689193748}
	if err := SetCursor(b, StreamEosio, want); err != nil {
		t.Fatal(err)
	}
	if err := SetCreationCount(b, 2531904); err != nil {
		t.Fatal(err)
	}
	if c, _ := GetCursor(b, StreamEosio); c != want {
		t.Errorf("cursor inside batch = %+v", c)
	}
	if c, _ := GetCursor(s.DB(), StreamEosio); c != (Cursor{}) {
		t.Errorf("cursor visible before commit: %+v", c)
	}
	if err := s.Commit(b); err != nil {
		t.Fatal(err)
	}

	if c, err := GetCursor(s.DB(), StreamEosio); err != nil || c != want {
		t.Errorf("eosio cursor = %+v, %v", c, err)
	}
	if c, err := GetCursor(s.DB(), StreamGift); err != nil || c != (Cursor{}) {
		t.Errorf("gift cursor moved with the eosio batch: %+v, %v", c, err)
	}
	if n, err := GetCreationCount(s.DB()); err != nil || n != 2531904 {
		t.Errorf("creation count = %d, %v", n, err)
	}
}
