package store

import "encoding/binary"

const (
	prefixMeta                byte = 0x00
	prefixCreation            byte = 0x01
	prefixCreatorIndex        byte = 0x02
	prefixDailyNetwork        byte = 0x03
	prefixDailyCreatorDay     byte = 0x04
	prefixDailyDayCreator     byte = 0x05
	prefixProgramEvent        byte = 0x06
	prefixProgramGiftDay      byte = 0x07
	prefixGiftAcct            byte = 0x08
	prefixProgramCreatorIndex byte = 0x09
	prefixGiftedBalance       byte = 0x0A
)

const (
	metaSchema        byte = 0x01
	metaCursorEosio   byte = 0x02
	metaCursorGift    byte = 0x03
	metaCreationCount byte = 0x04
)

const blockTimeEpoch = 946684800

func DayFromBlockTime(blockTime uint32) uint32 {
	return uint32((blockTimeEpoch + uint64(blockTime)/2) / 86400)
}

func schemaKey() []byte        { return []byte{prefixMeta, metaSchema} }
func creationCountKey() []byte { return []byte{prefixMeta, metaCreationCount} }

func cursorKey(s Stream) []byte {
	if s == StreamGift {
		return []byte{prefixMeta, metaCursorGift}
	}
	return []byte{prefixMeta, metaCursorEosio}
}

func key(prefix byte, parts ...any) []byte {
	buf := []byte{prefix}
	for _, p := range parts {
		switch v := p.(type) {
		case uint64:
			buf = binary.BigEndian.AppendUint64(buf, v)
		case uint32:
			buf = binary.BigEndian.AppendUint32(buf, v)
		default:
			panic("store: unsupported key part")
		}
	}
	return buf
}

func CreationKey(account uint64) []byte { return key(prefixCreation, account) }

func CreatorIndexKey(creator, seq uint64) []byte { return key(prefixCreatorIndex, creator, seq) }
func CreatorIndexPrefix(creator uint64) []byte   { return key(prefixCreatorIndex, creator) }

func DailyNetworkKey(day uint32) []byte { return key(prefixDailyNetwork, day) }

func DailyCreatorDayKey(creator uint64, day uint32) []byte {
	return key(prefixDailyCreatorDay, creator, day)
}
func DailyCreatorDayPrefix(creator uint64) []byte { return key(prefixDailyCreatorDay, creator) }

func DailyDayCreatorKey(day uint32, creator uint64) []byte {
	return key(prefixDailyDayCreator, day, creator)
}
func DailyDayCreatorPrefix(day uint32) []byte { return key(prefixDailyDayCreator, day) }

func ProgramEventKey(seq uint64) []byte { return key(prefixProgramEvent, seq) }
func ProgramEventPrefix() []byte        { return []byte{prefixProgramEvent} }

func ProgramGiftDayKey(day uint32, seq uint64) []byte { return key(prefixProgramGiftDay, day, seq) }
func ProgramGiftDayPrefix(day uint32) []byte          { return key(prefixProgramGiftDay, day) }

func GiftAcctKey(account uint64) []byte { return key(prefixGiftAcct, account) }

func ProgramCreatorIndexKey(programCreator, seq uint64) []byte {
	return key(prefixProgramCreatorIndex, programCreator, seq)
}
func ProgramCreatorIndexPrefix(programCreator uint64) []byte {
	return key(prefixProgramCreatorIndex, programCreator)
}

func GiftedBalanceKey(giftee uint64) []byte { return key(prefixGiftedBalance, giftee) }

func PrefixEnd(prefix []byte) []byte {
	end := append([]byte(nil), prefix...)
	for i := len(end) - 1; i >= 0; i-- {
		end[i]++
		if end[i] != 0 {
			return end[:i+1]
		}
	}
	return nil
}

func keyLen(k []byte, prefix byte, n int) bool { return len(k) == 1+n && k[0] == prefix }

func DecodeCreationKey(k []byte) (account uint64, ok bool) {
	if !keyLen(k, prefixCreation, 8) {
		return 0, false
	}
	return binary.BigEndian.Uint64(k[1:]), true
}

func DecodeCreatorIndexKey(k []byte) (creator, seq uint64, ok bool) {
	if !keyLen(k, prefixCreatorIndex, 16) {
		return 0, 0, false
	}
	return binary.BigEndian.Uint64(k[1:]), binary.BigEndian.Uint64(k[9:]), true
}

func DecodeDailyNetworkKey(k []byte) (day uint32, ok bool) {
	if !keyLen(k, prefixDailyNetwork, 4) {
		return 0, false
	}
	return binary.BigEndian.Uint32(k[1:]), true
}

func DecodeDailyCreatorDayKey(k []byte) (creator uint64, day uint32, ok bool) {
	if !keyLen(k, prefixDailyCreatorDay, 12) {
		return 0, 0, false
	}
	return binary.BigEndian.Uint64(k[1:]), binary.BigEndian.Uint32(k[9:]), true
}

func DecodeDailyDayCreatorKey(k []byte) (day uint32, creator uint64, ok bool) {
	if !keyLen(k, prefixDailyDayCreator, 12) {
		return 0, 0, false
	}
	return binary.BigEndian.Uint32(k[1:]), binary.BigEndian.Uint64(k[5:]), true
}

func DecodeProgramEventKey(k []byte) (seq uint64, ok bool) {
	if !keyLen(k, prefixProgramEvent, 8) {
		return 0, false
	}
	return binary.BigEndian.Uint64(k[1:]), true
}

func DecodeProgramGiftDayKey(k []byte) (day uint32, seq uint64, ok bool) {
	if !keyLen(k, prefixProgramGiftDay, 12) {
		return 0, 0, false
	}
	return binary.BigEndian.Uint32(k[1:]), binary.BigEndian.Uint64(k[5:]), true
}

func DecodeGiftAcctKey(k []byte) (account uint64, ok bool) {
	if !keyLen(k, prefixGiftAcct, 8) {
		return 0, false
	}
	return binary.BigEndian.Uint64(k[1:]), true
}

func DecodeProgramCreatorIndexKey(k []byte) (programCreator, seq uint64, ok bool) {
	if !keyLen(k, prefixProgramCreatorIndex, 16) {
		return 0, 0, false
	}
	return binary.BigEndian.Uint64(k[1:]), binary.BigEndian.Uint64(k[9:]), true
}

func DecodeGiftedBalanceKey(k []byte) (giftee uint64, ok bool) {
	if !keyLen(k, prefixGiftedBalance, 8) {
		return 0, false
	}
	return binary.BigEndian.Uint64(k[1:]), true
}
