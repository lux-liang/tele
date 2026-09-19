package tg

import (
	"context"

	"github.com/gotd/td/telegram/updates"
	"github.com/gotd/td/tg"
	"go.uber.org/zap"
)

// channelDiffTimeoutFlag is the flag bit carrying the optional timeout field on
// every updates.channelDifference* constructor. GetTimeout reports the field as
// present from this bit alone, so zeroing the value does not clear it.
const channelDiffTimeoutFlag = 1

// maxChannelDiffConcurrency caps how many getChannelDifference calls the
// updates manager keeps in flight across all channels. It replaces the pacing
// the stripped timeout used to provide, without the five-minute stalls: bursts
// stay bounded while any single gap is still closed at once.
//
// Sized against the shape of a real account. A member of a few dozen channels
// fires one call per channel at startup, and each takes roughly a second, so a
// smaller cap would show up as startup latency while a larger one buys nothing.
const maxChannelDiffConcurrency = 8

// channelDiffAPI sits between updates.Manager and the raw API and strips the
// timeout field from every updates.getChannelDifference response.
//
// Telegram attaches a timeout to a channel difference, 300 seconds for a busy
// supergroup. gotd reads it as a ban on calling getChannelDifference again for
// that channel, and while the ban holds it discards every update the channel
// receives. A pts gap that opens inside the window therefore freezes the chat
// for the rest of it, with no callback and no error: one debug line and
// nothing else. Each successful catch-up re-arms the ban, so a busy supergroup
// cycles between five minutes of silence and a burst of backlog (#266).
//
// The TL schema defines the field as "clients are supposed to refetch the
// channel difference after timeout seconds have elapsed", a polling hint for an
// idle channel rather than a bar on recovering a gap. With it gone gotd closes
// the gap on its own 500ms gap timer, the way it already does for channels
// whose difference carries no timeout at all.
//
// Only the timeout is touched. The difference itself reaches the manager
// unchanged, the too-long variant included, so the gap repair behind it still
// runs (#262).
//
// Upstream fix: gotd/td#1852. Delete this when it ships in a release, and the
// same for the entry it has in docs/gotd-workarounds.md.
// maxChannelDiffConcurrency stays either way: it bounds the startup burst on
// its own merits.
type channelDiffAPI struct {
	updates.API
	log *zap.Logger
}

func newChannelDiffAPI(api updates.API, log *zap.Logger) *channelDiffAPI {
	return &channelDiffAPI{API: api, log: log}
}

func (a *channelDiffAPI) UpdatesGetChannelDifference(
	ctx context.Context,
	req *tg.UpdatesGetChannelDifferenceRequest,
) (tg.UpdatesChannelDifferenceClass, error) {
	diff, err := a.API.UpdatesGetChannelDifference(ctx, req)
	if err != nil {
		return diff, err
	}

	timeout, had := clearChannelDiffTimeout(diff)
	if had {
		a.log.Debug("dropped the channel difference cooldown",
			zap.Int64("channel_id", inputChannelID(req.Channel)),
			zap.Int("timeout", timeout),
		)
	}
	return diff, nil
}

// clearChannelDiffTimeout unsets the optional timeout on any of the three
// difference constructors, reporting what was there. A nil or unknown response
// is left alone.
func clearChannelDiffTimeout(diff tg.UpdatesChannelDifferenceClass) (timeout int, had bool) {
	switch d := diff.(type) {
	case *tg.UpdatesChannelDifference:
		timeout, had = d.GetTimeout()
		d.Flags.Unset(channelDiffTimeoutFlag)
		d.Timeout = 0
	case *tg.UpdatesChannelDifferenceEmpty:
		timeout, had = d.GetTimeout()
		d.Flags.Unset(channelDiffTimeoutFlag)
		d.Timeout = 0
	case *tg.UpdatesChannelDifferenceTooLong:
		timeout, had = d.GetTimeout()
		d.Flags.Unset(channelDiffTimeoutFlag)
		d.Timeout = 0
	}
	return timeout, had
}

// inputChannelID digs the channel out of the request for the log line only.
// inputChannelEmpty carries no ID and yields zero.
func inputChannelID(c tg.InputChannelClass) int64 {
	ch, ok := c.AsNotEmpty()
	if !ok {
		return 0
	}
	return ch.GetChannelID()
}
