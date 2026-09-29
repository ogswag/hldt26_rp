package ops

import (
	"errors"
	"strings"
)

// Order keys follow the fractional-indexing npm package: base62 digits, an integer part whose length
// is set by the head letter, then a fraction without a trailing zero. Keys compare as plain strings.

const (
	orderDigits    = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	maxOrderKeyLen = 128
)

var errOrderKey = errors.New("ops.order: invalid key")

// ValidOrderKey reports whether key is a well-formed order key.
func ValidOrderKey(key string) bool {
	if key == "" || len(key) > maxOrderKeyLen {
		return false
	}
	for i := 0; i < len(key); i++ {
		if strings.IndexByte(orderDigits, key[i]) < 0 {
			return false
		}
	}
	return validateOrderKey(key) == nil
}

func integerLength(head byte) (int, error) {
	switch {
	case head >= 'a' && head <= 'z':
		return int(head-'a') + 2, nil
	case head >= 'A' && head <= 'Z':
		return int('Z'-head) + 2, nil
	}
	return 0, errOrderKey
}

func integerPart(key string) (string, error) {
	if key == "" {
		return "", errOrderKey
	}
	n, err := integerLength(key[0])
	if err != nil || n > len(key) {
		return "", errOrderKey
	}
	return key[:n], nil
}

func validateOrderKey(key string) error {
	if key == "A"+strings.Repeat("0", 26) {
		return errOrderKey
	}
	i, err := integerPart(key)
	if err != nil {
		return err
	}
	if f := key[len(i):]; strings.HasSuffix(f, "0") {
		return errOrderKey
	}
	return nil
}

func midpoint(a, b string, hasB bool) (string, error) {
	if hasB && a >= b {
		return "", errOrderKey
	}
	if strings.HasSuffix(a, "0") || (hasB && strings.HasSuffix(b, "0")) {
		return "", errOrderKey
	}
	if hasB {
		n := 0
		for {
			ca := byte('0')
			if n < len(a) {
				ca = a[n]
			}
			if n >= len(b) || ca != b[n] {
				break
			}
			n++
		}
		if n > 0 {
			rest, err := midpoint(tail(a, n), b[n:], true)
			if err != nil {
				return "", err
			}
			return b[:n] + rest, nil
		}
	}
	digitA := 0
	if a != "" {
		digitA = strings.IndexByte(orderDigits, a[0])
	}
	digitB := len(orderDigits)
	if hasB && b != "" {
		digitB = strings.IndexByte(orderDigits, b[0])
	}
	if digitB-digitA > 1 {
		mid := (digitA + digitB + 1) / 2
		return orderDigits[mid : mid+1], nil
	}
	if hasB && len(b) > 1 {
		return b[:1], nil
	}
	rest, err := midpoint(tail(a, 1), "", false)
	if err != nil {
		return "", err
	}
	return orderDigits[digitA:digitA+1] + rest, nil
}

func tail(s string, n int) string {
	if n >= len(s) {
		return ""
	}
	return s[n:]
}

func incrementInteger(x string) (string, bool, error) {
	if n, err := integerLength(x[0]); err != nil || n != len(x) {
		return "", false, errOrderKey
	}
	head := x[0]
	digs := []byte(x[1:])
	carry := true
	for i := len(digs) - 1; carry && i >= 0; i-- {
		d := strings.IndexByte(orderDigits, digs[i]) + 1
		if d == len(orderDigits) {
			digs[i] = orderDigits[0]
		} else {
			digs[i] = orderDigits[d]
			carry = false
		}
	}
	if !carry {
		return string(head) + string(digs), true, nil
	}
	if head == 'Z' {
		return "a" + orderDigits[:1], true, nil
	}
	if head == 'z' {
		return "", false, nil
	}
	h := head + 1
	if h > 'a' {
		digs = append(digs, orderDigits[0])
	} else {
		digs = digs[:len(digs)-1]
	}
	return string(h) + string(digs), true, nil
}

func decrementInteger(x string) (string, bool, error) {
	if n, err := integerLength(x[0]); err != nil || n != len(x) {
		return "", false, errOrderKey
	}
	head := x[0]
	digs := []byte(x[1:])
	borrow := true
	for i := len(digs) - 1; borrow && i >= 0; i-- {
		d := strings.IndexByte(orderDigits, digs[i]) - 1
		if d == -1 {
			digs[i] = orderDigits[len(orderDigits)-1]
		} else {
			digs[i] = orderDigits[d]
			borrow = false
		}
	}
	if !borrow {
		return string(head) + string(digs), true, nil
	}
	if head == 'a' {
		return "Z" + orderDigits[len(orderDigits)-1:], true, nil
	}
	if head == 'A' {
		return "", false, nil
	}
	h := head - 1
	if h < 'Z' {
		digs = append(digs, orderDigits[len(orderDigits)-1])
	} else {
		digs = digs[:len(digs)-1]
	}
	return string(h) + string(digs), true, nil
}

// KeyBetween returns a key strictly between a and b. An empty a means the start, an empty b the end.
func KeyBetween(a, b string) (string, error) {
	if a != "" {
		if err := validateOrderKey(a); err != nil {
			return "", err
		}
	}
	if b != "" {
		if err := validateOrderKey(b); err != nil {
			return "", err
		}
	}
	if a != "" && b != "" && a >= b {
		return "", errOrderKey
	}
	if a == "" {
		if b == "" {
			return "a" + orderDigits[:1], nil
		}
		ib, _ := integerPart(b)
		fb := b[len(ib):]
		if ib == "A"+strings.Repeat("0", 26) {
			m, err := midpoint("", fb, true)
			return ib + m, err
		}
		if ib < b {
			return ib, nil
		}
		res, ok, err := decrementInteger(ib)
		if err != nil {
			return "", err
		}
		if !ok {
			return "", errOrderKey
		}
		return res, nil
	}
	ia, _ := integerPart(a)
	fa := a[len(ia):]
	if b == "" {
		i, ok, err := incrementInteger(ia)
		if err != nil {
			return "", err
		}
		if !ok {
			m, err := midpoint(fa, "", false)
			return ia + m, err
		}
		return i, nil
	}
	ib, _ := integerPart(b)
	fb := b[len(ib):]
	if ia == ib {
		m, err := midpoint(fa, fb, true)
		return ia + m, err
	}
	i, ok, err := incrementInteger(ia)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", errOrderKey
	}
	if i < b {
		return i, nil
	}
	m, err := midpoint(fa, "", false)
	return ia + m, err
}

// KeysAfter returns n increasing keys after a (or from the start when a is empty).
func KeysAfter(a string, n int) ([]string, error) {
	out := make([]string, 0, n)
	prev := a
	for i := 0; i < n; i++ {
		k, err := KeyBetween(prev, "")
		if err != nil {
			return nil, err
		}
		out = append(out, k)
		prev = k
	}
	return out, nil
}
