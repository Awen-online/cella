// Package bech32 decodes Bech32 strings as Cardano uses them (CIP-5): wallet
// addresses, stake addresses, and credentials.
//
// This is BIP-173's Bech32 with one deliberate difference: the 90-character
// length limit is not enforced. Cardano's base addresses are 103 characters, so
// a spec-strict BIP-173 decoder rejects every real mainnet address. CIP-5 drops
// the limit for exactly this reason.
//
// Only decoding is implemented — Cella reads addresses, it never mints them.
package bech32

import (
	"fmt"
	"strings"
)

// charset is the Bech32 alphabet. Note it deliberately excludes 1, b, i and o,
// which are easily confused with l, 6, 1 and 0.
const charset = "qpzry9x8gf2tvdw0s3jn54khce6mua7l"

// reverse maps a charset byte back to its 5-bit value; -1 means "not in the
// alphabet".
var reverse = func() [256]int8 {
	var r [256]int8
	for i := range r {
		r[i] = -1
	}
	for i, c := range charset {
		r[c] = int8(i)
	}
	return r
}()

// Decode splits a Bech32 string into its human-readable prefix and the data it
// carries, verifying the checksum. The returned bytes are the payload converted
// back from 5-bit groups to 8-bit — for a Cardano address, the raw address
// bytes.
func Decode(s string) (hrp string, data []byte, err error) {
	// Bech32 is case-insensitive but must not be mixed: a mixed-case string is
	// ambiguous under the checksum, so it is rejected rather than guessed at.
	lower, upper := strings.ToLower(s), strings.ToUpper(s)
	if s != lower && s != upper {
		return "", nil, fmt.Errorf("bech32: mixed case")
	}
	s = lower

	i := strings.LastIndexByte(s, '1')
	if i < 1 {
		return "", nil, fmt.Errorf("bech32: no separator")
	}
	hrp = s[:i]
	payload := s[i+1:]
	if len(payload) < 6 {
		return "", nil, fmt.Errorf("bech32: payload too short for a checksum")
	}

	values := make([]byte, 0, len(payload))
	for j := 0; j < len(payload); j++ {
		v := reverse[payload[j]]
		if v < 0 {
			return "", nil, fmt.Errorf("bech32: invalid character %q", payload[j])
		}
		values = append(values, byte(v))
	}

	if !verifyChecksum(hrp, values) {
		return "", nil, fmt.Errorf("bech32: bad checksum")
	}

	// Drop the 6-symbol checksum, then regroup 5-bit values into bytes.
	data, err = convertBits(values[:len(values)-6], 5, 8, false)
	if err != nil {
		return "", nil, err
	}
	return hrp, data, nil
}

// Encode renders a human-readable prefix and payload bytes as Bech32.
//
// Cella reads addresses rather than minting them, so this exists mainly to
// derive an address from a credential — for example turning a delegate's key
// hash back into the reward address it corresponds to.
func Encode(hrp string, data []byte) (string, error) {
	if hrp == "" {
		return "", fmt.Errorf("bech32: empty prefix")
	}
	values, err := convertBits(data, 8, 5, true)
	if err != nil {
		return "", err
	}

	// The checksum is computed over the payload followed by six zero symbols,
	// then xored with 1.
	chk := polymod(append(append(hrpExpand(hrp), values...), make([]byte, 6)...)) ^ 1

	var b strings.Builder
	b.WriteString(hrp)
	b.WriteByte('1')
	for _, v := range values {
		b.WriteByte(charset[v])
	}
	for i := 0; i < 6; i++ {
		b.WriteByte(charset[(chk>>uint(5*(5-i)))&31])
	}
	return b.String(), nil
}

// polymod is the Bech32 checksum function (BIP-173).
func polymod(values []byte) uint32 {
	gen := [5]uint32{0x3b6a57b2, 0x26508e6d, 0x1ea119fa, 0x3d4233dd, 0x2a1462b3}
	chk := uint32(1)
	for _, v := range values {
		top := chk >> 25
		chk = (chk&0x1ffffff)<<5 ^ uint32(v)
		for i := 0; i < 5; i++ {
			if (top>>uint(i))&1 == 1 {
				chk ^= gen[i]
			}
		}
	}
	return chk
}

// hrpExpand expands the human-readable part for checksumming.
func hrpExpand(hrp string) []byte {
	out := make([]byte, 0, len(hrp)*2+1)
	for i := 0; i < len(hrp); i++ {
		out = append(out, hrp[i]>>5)
	}
	out = append(out, 0)
	for i := 0; i < len(hrp); i++ {
		out = append(out, hrp[i]&31)
	}
	return out
}

func verifyChecksum(hrp string, values []byte) bool {
	return polymod(append(hrpExpand(hrp), values...)) == 1
}

// convertBits regroups a byte slice from `from`-bit groups into `to`-bit groups.
func convertBits(data []byte, from, to uint, pad bool) ([]byte, error) {
	var acc uint32
	var bits uint
	maxv := uint32(1<<to) - 1
	out := make([]byte, 0, len(data)*int(from)/int(to)+1)

	for _, b := range data {
		if uint32(b) >= 1<<from {
			return nil, fmt.Errorf("bech32: value %d overflows %d bits", b, from)
		}
		acc = acc<<from | uint32(b)
		bits += from
		for bits >= to {
			bits -= to
			out = append(out, byte(acc>>bits&maxv))
		}
	}

	if pad {
		if bits > 0 {
			out = append(out, byte(acc<<(to-bits)&maxv))
		}
	} else if bits >= from || acc<<(to-bits)&maxv != 0 {
		// Leftover bits must be zero padding, not discarded data.
		return nil, fmt.Errorf("bech32: non-zero padding")
	}
	return out, nil
}
