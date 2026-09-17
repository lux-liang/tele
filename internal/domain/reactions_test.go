package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/sorokin-vladimir/tele/internal/domain"
)

// The reaction trace (#248) compares sets written by different paths line by
// line, so one set must always read the same way, whoever logged it.
func TestFormatReactions(t *testing.T) {
	assert.Equal(t, "[]", domain.FormatReactions(nil))
	assert.Equal(t, "[]", domain.FormatReactions([]domain.Reaction{}))
	assert.Equal(t, "[👍:3:me 🔥:1]", domain.FormatReactions([]domain.Reaction{
		{Emoji: "👍", Count: 3, IsChosen: true},
		{Emoji: "🔥", Count: 1},
	}))
}

func TestSameReactions(t *testing.T) {
	a := []domain.Reaction{{Emoji: "👍", Count: 1, IsChosen: true}}

	assert.True(t, domain.SameReactions(nil, []domain.Reaction{}), "no reactions is one state, however it is spelled")
	assert.True(t, domain.SameReactions(a, []domain.Reaction{{Emoji: "👍", Count: 1, IsChosen: true}}))
	assert.False(t, domain.SameReactions(a, nil))
	assert.False(t, domain.SameReactions(a, []domain.Reaction{{Emoji: "👍", Count: 1}}), "losing our choice is a change")
	assert.False(t, domain.SameReactions(a, []domain.Reaction{{Emoji: "👍", Count: 2, IsChosen: true}}))
}
