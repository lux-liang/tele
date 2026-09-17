package tg

import (
	"strconv"
	"strings"

	"github.com/gotd/td/tg"
)

// formatTGReactions renders a raw reaction set for the reaction trace (#248).
// It keeps what convertReactions throws away - custom and paid reactions, and
// the chosen order rather than a flag - because the trace is there to show what
// Telegram actually sent.
func formatTGReactions(mr tg.MessageReactions) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, rc := range mr.Results {
		if i > 0 {
			b.WriteByte(' ')
		}
		switch r := rc.Reaction.(type) {
		case *tg.ReactionEmoji:
			b.WriteString(r.Emoticon)
		case *tg.ReactionCustomEmoji:
			b.WriteString("custom")
			b.WriteString(strconv.FormatInt(r.DocumentID, 10))
		case *tg.ReactionPaid:
			b.WriteString("paid")
		default:
			b.WriteString("?")
		}
		b.WriteByte(':')
		b.WriteString(strconv.Itoa(rc.Count))
		if order, ok := rc.GetChosenOrder(); ok {
			b.WriteString(":chosen=")
			b.WriteString(strconv.Itoa(order))
		}
	}
	b.WriteByte(']')
	return b.String()
}

// hasMyRecentReaction reports whether our own reaction is among the recent
// ones, which Telegram fills independently of the chosen order.
func hasMyRecentReaction(mr tg.MessageReactions) bool {
	for _, r := range mr.RecentReactions {
		if r.My {
			return true
		}
	}
	return false
}

// reactionsInReply lists the update types in the reply to messages.sendReaction
// and the reaction set it carries for msgID: "none" when no update in the reply
// is about that message, "absent" when one is but has no reactions field.
func reactionsInReply(reply tg.UpdatesClass, msgID int) (types []string, found string) {
	var upds []tg.UpdateClass
	switch r := reply.(type) {
	case *tg.Updates:
		upds = r.Updates
	case *tg.UpdatesCombined:
		upds = r.Updates
	case *tg.UpdateShort:
		upds = []tg.UpdateClass{r.Update}
	}
	found = "none"
	for _, u := range upds {
		types = append(types, u.TypeName())
		switch v := u.(type) {
		case *tg.UpdateMessageReactions:
			if v.MsgID == msgID {
				found = formatTGReactions(v.Reactions)
			}
		case *tg.UpdateEditMessage:
			if s, ok := editReactions(v.Message, msgID); ok {
				found = s
			}
		case *tg.UpdateEditChannelMessage:
			if s, ok := editReactions(v.Message, msgID); ok {
				found = s
			}
		}
	}
	return types, found
}

func editReactions(raw tg.MessageClass, msgID int) (string, bool) {
	m, ok := raw.(*tg.Message)
	if !ok || m.ID != msgID {
		return "", false
	}
	mr, ok := m.GetReactions()
	if !ok {
		return "absent", true
	}
	return formatTGReactions(mr), true
}
