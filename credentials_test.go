package main

import (
	"os"
	"path/filepath"
	"testing"
)

func setupCredentialsDir(t *testing.T) {
	t.Helper()
	credentialsDir = t.TempDir()
	t.Cleanup(func() { credentialsDir = "" })
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	setupCredentialsDir(t)

	creds := BridgeCredentials{Username: "user1", Clientkey: "key1"}
	if err := SaveCredentials("bridge-1", creds); err != nil {
		t.Fatalf("SaveCredentials: %v", err)
	}

	got, found, err := LoadCredentials("bridge-1")
	if err != nil {
		t.Fatalf("LoadCredentials: %v", err)
	}
	if !found {
		t.Fatal("expected credentials to be found")
	}
	if got.Username != "user1" || got.Clientkey != "key1" {
		t.Fatalf("got %+v, want username=user1 clientkey=key1", got)
	}
}

func TestLoadNonexistentFile(t *testing.T) {
	setupCredentialsDir(t)

	_, found, err := LoadCredentials("bridge-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Fatal("expected credentials not found")
	}
}

func TestLoadMissingBridgeID(t *testing.T) {
	setupCredentialsDir(t)

	if err := SaveCredentials("bridge-1", BridgeCredentials{Username: "u", Clientkey: "k"}); err != nil {
		t.Fatalf("SaveCredentials: %v", err)
	}

	_, found, err := LoadCredentials("bridge-other")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Fatal("expected credentials not found for missing bridge ID")
	}
}

func TestDeleteCredentials(t *testing.T) {
	setupCredentialsDir(t)

	if err := SaveCredentials("bridge-1", BridgeCredentials{Username: "u", Clientkey: "k"}); err != nil {
		t.Fatalf("SaveCredentials: %v", err)
	}

	if err := DeleteCredentials("bridge-1"); err != nil {
		t.Fatalf("DeleteCredentials: %v", err)
	}

	_, found, err := LoadCredentials("bridge-1")
	if err != nil {
		t.Fatalf("LoadCredentials: %v", err)
	}
	if found {
		t.Fatal("expected credentials to be deleted")
	}
}

func TestMultipleBridgesPreserved(t *testing.T) {
	setupCredentialsDir(t)

	c1 := BridgeCredentials{Username: "user1", Clientkey: "key1"}
	c2 := BridgeCredentials{Username: "user2", Clientkey: "key2"}

	if err := SaveCredentials("bridge-1", c1); err != nil {
		t.Fatalf("SaveCredentials bridge-1: %v", err)
	}
	if err := SaveCredentials("bridge-2", c2); err != nil {
		t.Fatalf("SaveCredentials bridge-2: %v", err)
	}

	got1, found, err := LoadCredentials("bridge-1")
	if err != nil || !found {
		t.Fatalf("LoadCredentials bridge-1: found=%v err=%v", found, err)
	}
	if got1.Username != "user1" {
		t.Fatalf("bridge-1: got username %q, want user1", got1.Username)
	}

	got2, found, err := LoadCredentials("bridge-2")
	if err != nil || !found {
		t.Fatalf("LoadCredentials bridge-2: found=%v err=%v", found, err)
	}
	if got2.Username != "user2" {
		t.Fatalf("bridge-2: got username %q, want user2", got2.Username)
	}

	// Delete one, other should remain
	if err := DeleteCredentials("bridge-1"); err != nil {
		t.Fatalf("DeleteCredentials: %v", err)
	}

	_, found, _ = LoadCredentials("bridge-1")
	if found {
		t.Fatal("bridge-1 should be deleted")
	}

	got2, found, _ = LoadCredentials("bridge-2")
	if !found {
		t.Fatal("bridge-2 should still exist")
	}
	if got2.Username != "user2" {
		t.Fatal("bridge-2 credentials changed unexpectedly")
	}
}

func TestSaveCreatesDirectory(t *testing.T) {
	tmp := t.TempDir()
	credentialsDir = filepath.Join(tmp, "nested", "dir")
	t.Cleanup(func() { credentialsDir = "" })

	if err := SaveCredentials("b1", BridgeCredentials{Username: "u", Clientkey: "k"}); err != nil {
		t.Fatalf("SaveCredentials: %v", err)
	}

	path := filepath.Join(credentialsDir, "credentials.json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("credentials file not created: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("file permissions: got %o, want 0600", info.Mode().Perm())
	}
}
