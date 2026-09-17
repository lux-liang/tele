package tg

import (
	"testing"

	"github.com/gotd/td/tg"
	"github.com/stretchr/testify/assert"
)

func TestFormatTGReactions(t *testing.T) {
	var rc tg.ReactionCount
	rc.SetChosenOrder(0)
	rc.Reaction = &tg.ReactionEmoji{Emoticon: "👍"}
	rc.Count = 2

	mr := tg.MessageReactions{Results: []tg.ReactionCount{
		rc,
		{Reaction: &tg.ReactionCustomEmoji{DocumentID: 42}, Count: 1},
		{Reaction: &tg.ReactionPaid{}, Count: 5},
	}}

	assert.Equal(t, "[👍:2:chosen=0 custom42:1 paid:5]", formatTGReactions(mr))
	assert.Equal(t, "[]", formatTGReactions(tg.MessageReactions{}))
}

func TestReactionsInReply(t *testing.T) {
	var rc tg.ReactionCount
	rc.SetChosenOrder(0)
	rc.Reaction = &tg.ReactionEmoji{Emoticon: "👍"}
	rc.Count = 1
	set := tg.MessageReactions{Results: []tg.ReactionCount{rc}}

	t.Run("updateMessageReactions for the message", func(t *testing.T) {
		reply := &tg.Updates{Updates: []tg.UpdateClass{
			&tg.UpdateMessageReactions{MsgID: 7, Reactions: set},
		}}
		types, found := reactionsInReply(reply, 7)
		assert.Equal(t, []string{"updateMessageReactions"}, types)
		assert.Equal(t, "[👍:1:chosen=0]", found)
	})

	t.Run("hidden edit of the message", func(t *testing.T) {
		msg := &tg.Message{ID: 7}
		msg.SetReactions(set)
		reply := &tg.Updates{Updates: []tg.UpdateClass{
			&tg.UpdateEditMessage{Message: msg},
		}}
		types, found := reactionsInReply(reply, 7)
		assert.Equal(t, []string{"updateEditMessage"}, types)
		assert.Equal(t, "[👍:1:chosen=0]", found)
	})

	t.Run("edit without a reactions field", func(t *testing.T) {
		reply := &tg.Updates{Updates: []tg.UpdateClass{
			&tg.UpdateEditMessage{Message: &tg.Message{ID: 7}},
		}}
		_, found := reactionsInReply(reply, 7)
		assert.Equal(t, "absent", found)
	})

	t.Run("nothing about the message", func(t *testing.T) {
		reply := &tg.UpdateShort{Update: &tg.UpdateMessageReactions{MsgID: 8, Reactions: set}}
		types, found := reactionsInReply(reply, 7)
		assert.Equal(t, []string{"updateMessageReactions"}, types)
		assert.Equal(t, "none", found)
	})
}
