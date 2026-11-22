package kql

// unicodeScalarEscape recognizes U plus eight hexadecimal digits without
// allocating. On malformed text, end leaves the first nonhex byte unconsumed
// so callers can still find the containing literal's closing delimiter.
func unicodeScalarEscape(source string, at int, budget *parseBudget) (end int, valid bool) {
	if at < 0 || at >= len(source) {
		return at, false
	}
	budget.step()
	if source[at] != 'U' {
		return at, false
	}
	end = at + 1
	var value uint32
	for digit := 0; digit < 8; digit++ {
		if end >= len(source) {
			return end, false
		}
		budget.step()
		c := source[end]
		var n uint32
		switch {
		case c >= '0' && c <= '9':
			n = uint32(c - '0')
		case c >= 'a' && c <= 'f':
			n = uint32(c-'a') + 10
		case c >= 'A' && c <= 'F':
			n = uint32(c-'A') + 10
		default:
			return end, false
		}
		value = value<<4 | n
		end++
	}
	return end, value <= 0x10ffff && (value < 0xd800 || value > 0xdfff)
}
