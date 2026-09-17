package domain

import (
	"strconv"
	"strings"
)

// FormatReactions renders a reaction set for the reaction trace (#248), with
// our own choice marked, so sets logged by different paths read the same.
func FormatReactions(r []Reaction) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, x := range r {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(x.Emoji)
		b.WriteByte(':')
		b.WriteString(strconv.Itoa(x.Count))
		if x.IsChosen {
			b.WriteString(":me")
		}
	}
	b.WriteByte(']')
	return b.String()
}

// SameReactions reports whether two sets are equal in order and content. A nil
// set and an empty one are the same state.
func SameReactions(a, b []Reaction) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
