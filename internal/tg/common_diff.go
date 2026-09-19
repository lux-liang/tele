package tg

import (
	"context"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/updates"
	"github.com/gotd/td/tg"
	"go.uber.org/zap"
)

// commonDiffAPI sits between updates.Manager and the raw API and delivers the
// updates a common difference recovers, itself, before handing that difference
// back to the manager untouched.
//
// The manager feeds a recovered difference back through its own pts sequence,
// then jumps the sequence to the state the difference ends at. Anything still
// buffered behind a gap at that moment is now behind the sequence, so the next
// flush discards it as outdated. The position keeps moving forward either way,
// which is why the account looks caught up while the changes themselves were
// dropped: a message rewritten repeatedly, as a bot streaming a reply does,
// ends up frozen at whatever it said before the gap (#267).
//
// Telegram also condenses a difference: a message edited thirty times comes
// back as one edit, not thirty. So the recovered edit is the only copy of the
// final text there will ever be, and losing it loses the message.
//
// Nothing is stripped and nothing is reordered. The manager still applies the
// difference its own way and still moves its position; we deliver the same
// updates in front of it, and applying by position makes the second copy a
// no-op rather than a second arrival (ADR 0016).
//
// Upstream fix: gotd/td#1854. Delete this when it ships in a release, and the
// same for the entry it has in docs/gotd-workarounds.md.
type commonDiffAPI struct {
	updates.API
	handler telegram.UpdateHandler
	log     *zap.Logger
}

func newCommonDiffAPI(api updates.API, handler telegram.UpdateHandler, log *zap.Logger) *commonDiffAPI {
	return &commonDiffAPI{API: api, handler: handler, log: log}
}

func (a *commonDiffAPI) UpdatesGetDifference(
	ctx context.Context,
	req *tg.UpdatesGetDifferenceRequest,
) (tg.UpdatesDifferenceClass, error) {
	diff, err := a.API.UpdatesGetDifference(ctx, req)
	if err != nil {
		return diff, err
	}

	// New messages are not in here. The manager hands those straight to the
	// handler already, and only the other updates go through the sequence that
	// drops them.
	switch d := diff.(type) {
	case *tg.UpdatesDifference:
		a.deliver(ctx, d.OtherUpdates, d.Users, d.Chats)
	case *tg.UpdatesDifferenceSlice:
		a.deliver(ctx, d.OtherUpdates, d.Users, d.Chats)
	}
	return diff, nil
}

// deliver hands the recovered updates to the dispatcher with the difference's
// own users and chats, which is what a handler needs to resolve their peers.
//
// It runs on the manager's goroutine, before the difference is handed back, so
// a handler that blocks holds up the catch-up. That is the same bargain the
// outbox hook already makes, and for the same reason: an event that is dropped
// to keep a queue moving is exactly the event this exists to save.
func (a *commonDiffAPI) deliver(
	ctx context.Context,
	upds []tg.UpdateClass,
	users []tg.UserClass,
	chats []tg.ChatClass,
) {
	if len(upds) == 0 {
		return
	}
	a.log.Debug("delivering the updates a common difference recovered",
		zap.Int("count", len(upds)),
		zap.Strings("updates", updateTypeNames(upds)),
	)
	if err := a.handler.Handle(ctx, &tg.UpdatesCombined{
		Updates: upds,
		Users:   users,
		Chats:   chats,
	}); err != nil {
		a.log.Warn("delivering a recovered difference failed", zap.Error(err))
	}
}
