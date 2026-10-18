//go:build unix

package config

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// TestSaveSettingsPreservesOwner: replacing the config file must not silently
// hand it to the service account (production runs the binary as root while the
// operator edits the file as their own user).
func TestSaveSettingsPreservesOwner(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs root to chown the fixture")
	}
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"admin_pass": "keep-me"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(path, 65534, 65534); err != nil {
		t.Skipf("chown fixture: %v", err)
	}

	if err := SaveSettings(path, Settings{
		SiteTitle:       "标题",
		LandingRemember: "30d",
	}); err != nil {
		t.Fatalf("save: %v", err)
	}

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatal("no stat_t")
	}
	if st.Uid != 65534 || st.Gid != 65534 {
		t.Errorf("owner = %d:%d, want 65534:65534", st.Uid, st.Gid)
	}
	if perm := fi.Mode().Perm(); perm != 0600 {
		t.Errorf("mode = %o, want 600", perm)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"admin_pass": "keep-me"`) {
		t.Errorf("admin_pass not preserved:\n%s", raw)
	}
}
