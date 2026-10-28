// Copyright 2026 Northern.tech AS
//
//    Licensed under the Apache License, Version 2.0 (the "License");
//    you may not use this file except in compliance with the License.
//    You may obtain a copy of the License at
//
//        http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS,
//    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//    See the License for the specific language governing permissions and
//    limitations under the License.

package server

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveAndLoadToken(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, ".cache")
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CACHE_HOME", cacheDir)

	err := SaveToken("test-jwt-token")
	if err != nil {
		t.Fatalf("SaveToken failed: %s", err)
	}

	token, err := LoadToken()
	if err != nil {
		t.Fatalf("LoadToken failed: %s", err)
	}
	if token != "test-jwt-token" {
		t.Errorf("got %q, want test-jwt-token", token)
	}
}

func TestLoadTokenNotFound(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(dir, ".cache"))

	_, err := LoadToken()
	if err == nil {
		t.Fatal("expected error for missing token, got nil")
	}
}

func TestLoadTokenEmpty(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, ".cache")
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CACHE_HOME", cacheDir)

	tokenDir := filepath.Join(cacheDir, "mender")
	os.MkdirAll(tokenDir, 0700)
	os.WriteFile(filepath.Join(tokenDir, "authtoken"), []byte("  \n"), 0600)

	_, err := LoadToken()
	if err == nil {
		t.Fatal("expected error for empty token, got nil")
	}
}

func TestResolveTokenWithFlag(t *testing.T) {
	token, err := ResolveToken("explicit-token")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if token != "explicit-token" {
		t.Errorf("got %q, want explicit-token", token)
	}
}

func TestResolveTokenFallsBackToStored(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, ".cache")
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CACHE_HOME", cacheDir)

	SaveToken("stored-token")

	token, err := ResolveToken("")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if token != "stored-token" {
		t.Errorf("got %q, want stored-token", token)
	}
}

func TestSaveTokenCreatesDir(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, "deep", "nested", ".cache")
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CACHE_HOME", cacheDir)

	err := SaveToken("deep-token")
	if err != nil {
		t.Fatalf("SaveToken failed: %s", err)
	}

	token, err := LoadToken()
	if err != nil {
		t.Fatalf("LoadToken failed: %s", err)
	}
	if token != "deep-token" {
		t.Errorf("got %q, want deep-token", token)
	}
}

func TestTokenPathXDGDefault(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CACHE_HOME", "")

	err := SaveToken("xdg-test")
	if err != nil {
		t.Fatalf("SaveToken failed: %s", err)
	}

	expected := filepath.Join(dir, ".cache", "mender", "authtoken")
	data, err := os.ReadFile(expected)
	if err != nil {
		t.Fatalf("token not at expected default path: %s", err)
	}
	if string(data) != "xdg-test" {
		t.Errorf("got %q, want xdg-test", string(data))
	}
}

func TestTokenMigrationSkippedWhenNewExists(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, ".cache")
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CACHE_HOME", cacheDir)

	oldDir := filepath.Join(dir, ".mender")
	os.MkdirAll(oldDir, 0700)
	os.WriteFile(
		filepath.Join(oldDir, "authtoken"),
		[]byte("old-token"), 0600,
	)

	newDir := filepath.Join(cacheDir, "mender")
	os.MkdirAll(newDir, 0700)
	os.WriteFile(
		filepath.Join(newDir, "authtoken"),
		[]byte("new-token"), 0600,
	)

	token, err := LoadToken()
	if err != nil {
		t.Fatalf("LoadToken failed: %s", err)
	}
	if token != "new-token" {
		t.Errorf("got %q, want new-token (migration should be skipped)", token)
	}
}

func TestResolveTokenNoStoredToken(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(dir, ".cache"))

	_, err := ResolveToken("")
	if err == nil {
		t.Fatal("expected error when no token stored and no flag")
	}
}

func TestLoadTokenReadError(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, ".cache")
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CACHE_HOME", cacheDir)

	tokenDir := filepath.Join(cacheDir, "mender")
	os.MkdirAll(tokenDir, 0700)
	tokenFile := filepath.Join(tokenDir, "authtoken")
	os.Mkdir(tokenFile, 0700)

	_, err := LoadToken()
	if err == nil {
		t.Fatal("expected error when token path is a directory")
	}
}

func TestTokenMigration(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, ".cache")
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CACHE_HOME", cacheDir)

	oldDir := filepath.Join(dir, ".mender")
	os.MkdirAll(oldDir, 0700)
	os.WriteFile(
		filepath.Join(oldDir, "authtoken"),
		[]byte("migrated-token"), 0600,
	)

	token, err := LoadToken()
	if err != nil {
		t.Fatalf("LoadToken failed: %s", err)
	}
	if token != "migrated-token" {
		t.Errorf("got %q, want migrated-token", token)
	}

	newPath := filepath.Join(cacheDir, "mender", "authtoken")
	if _, err := os.Stat(newPath); err != nil {
		t.Errorf("token not migrated to new path: %s", err)
	}
}
