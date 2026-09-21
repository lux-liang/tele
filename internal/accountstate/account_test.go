package accountstate_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gotd/td/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/accountstate"
)

func TestReconcile_MatchingIdentityPreservesAccountFiles(t *testing.T) {
	stateDir := t.TempDir()
	sessionFile := filepath.Join(stateDir, "session.json")
	writeSession(t, sessionFile, []byte("account1"))
	require.NoError(t, accountstate.Record(stateDir, sessionFile))
	writeFile(t, filepath.Join(stateDir, "state.db"), "private")
	mediaDir, avatarDir := seedCaches(t, stateDir)

	cleared, err := accountstate.Reconcile(stateDir, sessionFile)
	require.NoError(t, err)
	assert.False(t, cleared)
	assertFileExists(t, filepath.Join(stateDir, "state.db"))
	assertFileExists(t, filepath.Join(mediaDir, "media.bin"))
	assertFileExists(t, filepath.Join(avatarDir, "avatar.bin"))
}

func TestReconcile_ChangedIdentityRemovesOnlyAccountOwnedFiles(t *testing.T) {
	stateDir := t.TempDir()
	sessionFile := filepath.Join(stateDir, "session.json")
	writeSession(t, sessionFile, []byte("account1"))
	require.NoError(t, accountstate.Record(stateDir, sessionFile))
	writeSession(t, sessionFile, []byte("account2"))

	for _, name := range []string{"state.db", "state.db-wal", "state.db-shm", "state.db.backup"} {
		writeFile(t, filepath.Join(stateDir, name), "private")
	}
	writeFile(t, filepath.Join(stateDir, "config.yml"), "kept")
	writeFile(t, filepath.Join(stateDir, "tele.lock"), "kept")
	mediaDir, avatarDir := seedCaches(t, stateDir)

	cleared, err := accountstate.Reconcile(stateDir, sessionFile)
	require.NoError(t, err)
	assert.True(t, cleared)
	for _, name := range []string{"state.db", "state.db-wal", "state.db-shm", "state.db.backup"} {
		assert.NoFileExists(t, filepath.Join(stateDir, name))
	}
	assert.NoDirExists(t, mediaDir)
	assert.NoDirExists(t, avatarDir)
	assertFileExists(t, sessionFile)
	assertFileExists(t, filepath.Join(stateDir, "account.id"))
	recordedID, err := os.ReadFile(filepath.Join(stateDir, "account.id"))
	require.NoError(t, err)
	assert.Equal(t, "6163636f756e7431\n", string(recordedID), "startup must not claim the new session")
	assertFileExists(t, filepath.Join(stateDir, "config.yml"))
	assertFileExists(t, filepath.Join(stateDir, "tele.lock"))
}

func TestReconcile_MissingOrUnreadableIdentityClearsUpgradeState(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, stateDir, sessionFile string)
	}{
		{
			name: "missing account ID",
			setup: func(t *testing.T, _ string, sessionFile string) {
				writeSession(t, sessionFile, []byte("account1"))
			},
		},
		{
			name: "unreadable session",
			setup: func(t *testing.T, stateDir, sessionFile string) {
				writeFile(t, sessionFile, "not a session")
				writeFile(t, filepath.Join(stateDir, "account.id"), "01020304\n")
			},
		},
		{
			name: "missing session",
			setup: func(t *testing.T, stateDir, _ string) {
				writeFile(t, filepath.Join(stateDir, "account.id"), "01020304\n")
			},
		},
		{
			name: "unreadable account ID",
			setup: func(t *testing.T, stateDir, sessionFile string) {
				writeSession(t, sessionFile, []byte("account1"))
				writeFile(t, filepath.Join(stateDir, "account.id"), "not hex\n")
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stateDir := t.TempDir()
			sessionFile := filepath.Join(stateDir, "session.json")
			tc.setup(t, stateDir, sessionFile)
			writeFile(t, filepath.Join(stateDir, "state.db"), "legacy private data")

			cleared, err := accountstate.Reconcile(stateDir, sessionFile)
			require.NoError(t, err)
			assert.True(t, cleared)
			assert.NoFileExists(t, filepath.Join(stateDir, "state.db"))
		})
	}
}

func TestRecordUsesCurrentSessionEveryTime(t *testing.T) {
	stateDir := t.TempDir()
	sessionFile := filepath.Join(stateDir, "session.json")
	writeSession(t, sessionFile, []byte("account1"))
	require.NoError(t, accountstate.Record(stateDir, sessionFile))
	first, err := os.ReadFile(filepath.Join(stateDir, "account.id"))
	require.NoError(t, err)

	writeSession(t, sessionFile, []byte("account2"))
	require.NoError(t, accountstate.Record(stateDir, sessionFile))
	second, err := os.ReadFile(filepath.Join(stateDir, "account.id"))
	require.NoError(t, err)

	assert.Equal(t, "6163636f756e7431\n", string(first))
	assert.Equal(t, "6163636f756e7432\n", string(second))
}

func TestReconcileReportsThePathItCouldNotClear(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "not-a-directory")
	writeFile(t, statePath, "blocked")

	cleared, err := accountstate.Reconcile(statePath, filepath.Join(statePath, "session.json"))
	assert.False(t, cleared)
	require.Error(t, err)
	assert.Contains(t, err.Error(), statePath)
}

func writeSession(t *testing.T, path string, authKeyID []byte) {
	t.Helper()
	loader := session.Loader{Storage: &session.FileStorage{Path: path}}
	require.NoError(t, loader.Save(context.Background(), &session.Data{AuthKeyID: authKeyID}))
}

func seedCaches(t *testing.T, stateDir string) (string, string) {
	t.Helper()
	mediaDir, err := accountstate.MediaCacheDir(stateDir)
	require.NoError(t, err)
	avatarDir, err := accountstate.AvatarCacheDir(stateDir)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.RemoveAll(filepath.Dir(mediaDir))
	})
	writeFile(t, filepath.Join(mediaDir, "media.bin"), "private")
	writeFile(t, filepath.Join(avatarDir, "avatar.bin"), "private")
	return mediaDir, avatarDir
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
	require.NoError(t, os.WriteFile(path, []byte(contents), 0600))
}

func assertFileExists(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.False(t, info.IsDir())
}
