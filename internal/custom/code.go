package custom

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/weather"
)

// A share code is "AB1-" and the Crockford base32 of a fixed binary layout.
//
// Version 1, frozen:
//
//	version   u8   (1)
//	base      u8   city.Map
//	seed      u64  little-endian
//	sliders   u8 each, in the order of the table: the step plus padOffset
//	flags     u8   bit 0 unsupported, bit 1 coast
//	weather   u8   weather.Setting
//	crc       u8   CRC-8 (polynomial 0x07) of every byte before it
//
// A later version only appends fields between the weather byte and the
// checksum; it never moves or re-means one. The decoder keeps a reader for
// every version, which reads the fields it knows and leaves the rest as the
// base preset has them, so every code ever shared goes on decoding. The
// golden code in code_test.go is held to that.
const (
	prefix = "AB1-"
	// padOffset is added to a step so that a map allowed to go unsupported may
	// put a slider below its first step. MaxPad is how far below.
	padOffset = 32
	MaxPad    = padOffset

	flagUnsupported = 1 << 0
	flagCoast       = 1 << 1

	// version is the layout Encode writes.
	version = 1
)

// ErrBadCode is wrapped by every reason a code is refused.
var ErrBadCode = errors.New("custom: not a map code")

const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

func crc8(b []byte) byte {
	var crc byte
	for _, x := range b {
		crc ^= x
		for range 8 {
			if crc&0x80 != 0 {
				crc = crc<<1 ^ 0x07
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}

// Encode returns the share code of a map.
func (s Spec) Encode() string {
	s = s.Normalize()
	b := make([]byte, 0, 32)
	b = append(b, version, byte(s.Base))
	b = binary.LittleEndian.AppendUint64(b, s.Seed)
	for _, st := range s.Step {
		b = append(b, byte(st+padOffset))
	}
	var flags byte
	if s.Unsupported {
		flags |= flagUnsupported
	}
	if s.Coast {
		flags |= flagCoast
	}
	b = append(b, flags, byte(s.Weather))
	b = append(b, crc8(b))
	return prefix + base32(b)
}

func base32(b []byte) string {
	var sb strings.Builder
	var acc, n uint
	for _, x := range b {
		acc, n = acc<<8|uint(x), n+8
		for n >= 5 {
			sb.WriteByte(crockford[acc>>(n-5)&31])
			n -= 5
		}
	}
	if n > 0 {
		sb.WriteByte(crockford[acc<<(5-n)&31])
	}
	return sb.String()
}

func unbase32(s string) ([]byte, bool) {
	var out []byte
	var acc, n uint
	for _, r := range strings.ToUpper(s) {
		switch r {
		case '-', ' ':
			continue
		case 'O':
			r = '0'
		case 'I', 'L':
			r = '1'
		}
		v := strings.IndexRune(crockford, r)
		if v < 0 {
			return nil, false
		}
		acc, n = acc<<5|uint(v), n+5
		if n >= 8 {
			out = append(out, byte(acc>>(n-8)))
			n -= 8
		}
	}
	// What is left over is padding, and must be zero.
	if acc&(1<<n-1) != 0 {
		return nil, false
	}
	return out, true
}

// readers decode each version's payload, which is what is between the version
// byte and the checksum. A reader is handed a spec already holding the base
// preset's own values, and fills in the fields its version has.
var readers = map[byte]func(s *Spec, p []byte) error{
	1: readV1,
}

// v1Len is the length of a version 1 payload, version byte and all.
const v1Len = 2 + 8 + NumSliders + 2

func readV1(s *Spec, p []byte) error {
	if len(p) != v1Len {
		return fmt.Errorf("%w: wrong length", ErrBadCode)
	}
	s.Seed = binary.LittleEndian.Uint64(p[2:10])
	for i := range s.Step {
		s.Step[i] = int(p[10+i]) - padOffset
	}
	flags := p[10+NumSliders]
	s.Unsupported = flags&flagUnsupported != 0
	s.Coast = flags&flagCoast != 0
	w := weather.Setting(p[11+NumSliders])
	if w > weather.Drift {
		return fmt.Errorf("%w: unknown weather", ErrBadCode)
	}
	s.Weather = w
	return nil
}

// Decode reads a share code. It is forgiving about case, dashes, spaces and the
// look-alike letters Crockford's alphabet folds together, and strict about
// everything else: a code that fails its checksum, names no map, or is of a
// version this build does not know is refused.
func Decode(code string) (Spec, error) {
	code = strings.TrimSpace(code)
	if len(code) <= len(prefix) || !strings.EqualFold(code[:3], prefix[:3]) ||
		(code[3] != '-' && code[3] != ' ') {
		return Spec{}, fmt.Errorf("%w: it should start with %s", ErrBadCode, prefix)
	}
	b, ok := unbase32(code[len(prefix):])
	if !ok || len(b) < 3 {
		return Spec{}, fmt.Errorf("%w: unreadable", ErrBadCode)
	}
	body, sum := b[:len(b)-1], b[len(b)-1]
	if crc8(body) != sum {
		return Spec{}, fmt.Errorf("%w: the checksum is wrong, so it has been mistyped", ErrBadCode)
	}
	read, ok := readers[body[0]]
	if !ok {
		return Spec{}, fmt.Errorf("%w: version %d is newer than this game", ErrBadCode, body[0])
	}
	if len(body) < 2 || !city.Map(body[1]).Valid() {
		return Spec{}, fmt.Errorf("%w: no such map", ErrBadCode)
	}
	s := FromMap(city.Map(body[1]), 0)
	if err := read(&s, body); err != nil {
		return Spec{}, err
	}
	return s.Normalize(), nil
}
