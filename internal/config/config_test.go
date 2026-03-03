package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadNonexistent(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Accounts) != 0 {
		t.Fatalf("got %d accounts, want 0", len(cfg.Accounts))
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfg, _ := Load()
	cfg.Set(AccountConfig{
		RefreshToken: "tok_abc",
		GuardianID:   "12345",
	})
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	// Verify file was created
	p := filepath.Join(dir, "parro", "config.json")
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("config file not created: %v", err)
	}

	// Load again and check
	cfg2, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	acct, ok := cfg2.Get("12345")
	if !ok {
		t.Fatal("account not found after reload")
	}
	if acct.RefreshToken != "tok_abc" {
		t.Fatalf("refresh_token = %q, want tok_abc", acct.RefreshToken)
	}
}

func TestGetMissing(t *testing.T) {
	cfg := &Config{Accounts: map[string]AccountConfig{}}
	_, ok := cfg.Get("nonexistent")
	if ok {
		t.Fatal("expected ok=false for missing account")
	}
}

func TestOnlyZeroAccounts(t *testing.T) {
	cfg := &Config{Accounts: map[string]AccountConfig{}}
	_, err := cfg.Only()
	if err == nil {
		t.Fatal("expected error with 0 accounts")
	}
}

func TestOnlyOneAccount(t *testing.T) {
	cfg := &Config{Accounts: map[string]AccountConfig{
		"111": {RefreshToken: "tok", GuardianID: "111"},
	}}
	acct, err := cfg.Only()
	if err != nil {
		t.Fatal(err)
	}
	if acct.GuardianID != "111" {
		t.Fatalf("got %q, want 111", acct.GuardianID)
	}
}

func TestOnlyTwoAccounts(t *testing.T) {
	cfg := &Config{Accounts: map[string]AccountConfig{
		"111": {RefreshToken: "tok1", GuardianID: "111"},
		"222": {RefreshToken: "tok2", GuardianID: "222"},
	}}
	_, err := cfg.Only()
	if err == nil {
		t.Fatal("expected error with 2 accounts")
	}
}

func TestListSorted(t *testing.T) {
	cfg := &Config{Accounts: map[string]AccountConfig{
		"333": {RefreshToken: "tok3", GuardianID: "333"},
		"111": {RefreshToken: "tok1", GuardianID: "111"},
		"222": {RefreshToken: "tok2", GuardianID: "222"},
	}}
	list := cfg.List()
	if len(list) != 3 {
		t.Fatalf("got %d, want 3", len(list))
	}
	if list[0].GuardianID != "111" || list[1].GuardianID != "222" || list[2].GuardianID != "333" {
		t.Fatalf("not sorted: %v", list)
	}
}

func TestFindByGuardianID(t *testing.T) {
	cfg := &Config{Accounts: map[string]AccountConfig{
		"111": {RefreshToken: "tok", GuardianID: "111", Username: "alice@school.nl"},
	}}
	acct, ok := cfg.Find("111")
	if !ok {
		t.Fatal("expected to find by guardian ID")
	}
	if acct.Username != "alice@school.nl" {
		t.Fatalf("got %q", acct.Username)
	}
}

func TestFindByUsername(t *testing.T) {
	cfg := &Config{Accounts: map[string]AccountConfig{
		"111": {RefreshToken: "tok", GuardianID: "111", Username: "alice@school.nl"},
		"222": {RefreshToken: "tok2", GuardianID: "222", Username: "bob@school.nl"},
	}}
	acct, ok := cfg.Find("bob@school.nl")
	if !ok {
		t.Fatal("expected to find by username")
	}
	if acct.GuardianID != "222" {
		t.Fatalf("got guardian %q, want 222", acct.GuardianID)
	}
}

func TestFindMissing(t *testing.T) {
	cfg := &Config{Accounts: map[string]AccountConfig{
		"111": {RefreshToken: "tok", GuardianID: "111", Username: "alice@school.nl"},
	}}
	_, ok := cfg.Find("nobody")
	if ok {
		t.Fatal("expected ok=false for missing account")
	}
}

func TestSetOverwrite(t *testing.T) {
	cfg := &Config{Accounts: map[string]AccountConfig{
		"111": {RefreshToken: "old", GuardianID: "111"},
	}}
	cfg.Set(AccountConfig{RefreshToken: "new", GuardianID: "111"})
	acct, ok := cfg.Get("111")
	if !ok {
		t.Fatal("account missing")
	}
	if acct.RefreshToken != "new" {
		t.Fatalf("got %q, want new", acct.RefreshToken)
	}
}
