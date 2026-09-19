package tg

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/updates"
	"github.com/gotd/td/tg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/sorokin-vladimir/tele/internal/store"
)

// The stand for the outbox hook. The peer reads our messages while a pts gap is
// open, so the read receipt arrives on the wire behind the gap. The catch-up
// that closes the gap does not mention it, and the manager then jumps past
// everything it buffered. The manager is real; only the API is faked.
const outboxStandMaxID = 7

// seenHandler stands in for the dispatcher and records every update the
// manager hands on. The manager calls it from its own goroutines.
type seenHandler struct {
	mu  sync.Mutex
	got []tg.UpdateClass
}

var _ telegram.UpdateHandler = (*seenHandler)(nil)

func (h *seenHandler) Handle(_ context.Context, u tg.UpdatesClass) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	switch u := u.(type) {
	case *tg.Updates:
		h.got = append(h.got, u.Updates...)
	case *tg.UpdatesCombined:
		h.got = append(h.got, u.Updates...)
	case *tg.UpdateShort:
		h.got = append(h.got, u.Update)
	}
	return nil
}

func (h *seenHandler) sawOutboxRead() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.ContainsFunc(h.got, func(u tg.UpdateClass) bool {
		_, ok := u.(*tg.UpdateReadHistoryOutbox)
		return ok
	})
}

func outboxRead(pts int) *tg.Updates {
	return &tg.Updates{
		Updates: []tg.UpdateClass{&tg.UpdateReadHistoryOutbox{
			Peer: &tg.PeerUser{UserID: standChatID}, MaxID: outboxStandMaxID, Pts: pts, PtsCount: 1,
		}},
		Users: standUser(),
		Date:  int(time.Now().Unix()),
	}
}

// outboxStand starts the real manager against a faked API whose catch-up
// recovers nothing but moves the position past the gap. hook says whether the
// wire reaches the manager through the outbox hook or straight.
func outboxStand(t *testing.T, hook bool) (telegram.UpdateHandler, *standAPI, *seenHandler, chan store.Event) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	seen := &seenHandler{}
	api := &standAPI{diff: &tg.UpdatesDifference{
		State: tg.UpdatesState{Pts: standEndPts, Date: int(time.Now().Unix())},
	}}
	manager := updates.New(updates.Config{Handler: seen})
	started := make(chan struct{})
	go func() {
		_ = manager.Run(ctx, api, 1, updates.AuthOptions{
			OnStart: func(context.Context) { close(started) },
		})
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("the updates manager never started")
	}
	require.Eventually(t, func() bool { return api.asks() > 0 }, 5*time.Second, 10*time.Millisecond,
		"the manager never ran its startup catch-up")
	api.arm()

	mustDeliver := make(chan store.Event, 4)
	var wire telegram.UpdateHandler = manager
	if hook {
		wire = newOutboxHook(manager, mustDeliver, zap.NewNop())
	}
	return wire, api, seen, mustDeliver
}

// readBehindAGap delivers one update in sequence, then the read receipt with
// pts that skip ahead, so the receipt itself is what reveals the gap and waits
// in the manager's buffer.
func readBehindAGap(t *testing.T, wire telegram.UpdateHandler) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, wire.Handle(ctx, standEdit(standStartTS+1, "hello")))
	require.NoError(t, wire.Handle(ctx, outboxRead(standStartTS+6)))
}

func TestStand_AnOutboxReadBehindAGapStillArrives(t *testing.T) {
	wire, _, _, mustDeliver := outboxStand(t, true)

	readBehindAGap(t, wire)

	select {
	case evt := <-mustDeliver:
		assert.Equal(t, store.EventReadOutbox, evt.Kind)
		assert.Equal(t, standChatID, evt.ChatID)
		assert.Equal(t, outboxStandMaxID, evt.ReadMaxID)
	case <-time.After(5 * time.Second):
		t.Fatal("the read receipt must be delivered even though the manager drops it")
	}
}

// The same stand with the wire going straight to the manager, which is what
// gotd does on its own. It reproduces the defect: the receipt waits behind the
// gap, the catch-up moves the position past it, and the manager throws the
// buffer away as outdated. The sender never sees their message marked read.
//
// When this test starts failing, gotd delivers what it buffered behind a gap:
// check whether the hook is still needed, then delete it, this test, and the
// entry both have in docs/gotd-workarounds.md (#270).
func TestStand_WithoutTheHookAnOutboxReadBehindAGapIsLost(t *testing.T) {
	wire, api, seen, _ := outboxStand(t, false)

	readBehindAGap(t, wire)

	require.Eventually(t, func() bool {
		return slices.Contains(api.askedFrom(), standStartTS+1)
	}, 5*time.Second, 20*time.Millisecond, "the gap was never chased")
	// Long enough for the catch-up to finish and for anything the manager was
	// going to release from its buffer to have been handed on.
	time.Sleep(time.Second)

	assert.False(t, seen.sawOutboxRead(),
		"upstream no longer drops updates buffered behind a gap; check whether the hook can go")
}
