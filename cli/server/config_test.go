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
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPersistServerToConfigCreatesNewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".mender-clirc")

	written, err := PersistServerToConfig(path, "https://example.com")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if !written {
		t.Fatal("expected written=true for a new file")
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("failed to stat created file: %s", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("expected file mode 0600, got %o", perm)
	}

	content := readJSONFile(t, path)
	if len(content) != 1 {
		t.Errorf("expected 1 key, got %d: %v", len(content), content)
	}
	if content["server"] != "https://example.com" {
		t.Errorf("expected server=https://example.com, got %v", content["server"])
	}
}

func TestPersistServerToConfigCreatesParentDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", ".mender-clirc")

	written, err := PersistServerToConfig(path, "https://example.com")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if !written {
		t.Fatal("expected written=true for a new file")
	}

	content := readJSONFile(t, path)
	if content["server"] != "https://example.com" {
		t.Errorf("expected server=https://example.com, got %v", content["server"])
	}
}

func TestPersistServerToConfigPreservesExistingKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".mender-clirc")

	original := []byte(`{"username": "alice", "password": "secret"}`)
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatalf("failed to write fixture file: %s", err)
	}

	written, err := PersistServerToConfig(path, "https://example.com")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if !written {
		t.Fatal("expected written=true when server key was absent")
	}

	content := readJSONFile(t, path)
	if len(content) != 3 {
		t.Errorf("expected 3 keys, got %d: %v", len(content), content)
	}
	if content["server"] != "https://example.com" {
		t.Errorf("expected server=https://example.com, got %v", content["server"])
	}
	if content["username"] != "alice" {
		t.Errorf("expected username=alice, got %v", content["username"])
	}
}

func TestPersistServerToConfigDoesNotOverwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".mender-clirc")

	original := []byte(`{"server": "https://staging.example.com", "username": "bob"}`)
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatalf("failed to write fixture file: %s", err)
	}

	written, err := PersistServerToConfig(path, "https://different.example.com")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if written {
		t.Fatal("expected written=false when server key already exists")
	}

	content := readJSONFile(t, path)
	if content["server"] != "https://staging.example.com" {
		t.Errorf("expected server to remain unchanged, got %v", content["server"])
	}
}

func TestPersistServerToConfigEmptyServerCounts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".mender-clirc")

	original := []byte(`{"server": ""}`)
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatalf("failed to write fixture file: %s", err)
	}

	written, err := PersistServerToConfig(path, "https://example.com")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if written {
		t.Fatal("expected written=false when server key is present (even if empty)")
	}
}

func TestPersistServerToConfigMalformedJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".mender-clirc")

	original := []byte(`{not valid json`)
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatalf("failed to write fixture file: %s", err)
	}

	_, err := PersistServerToConfig(path, "https://example.com")
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %s", err)
	}
	if string(data) != string(original) {
		t.Errorf("expected file to remain unchanged, got %q", string(data))
	}
}

func TestResolveConfigFilePath(t *testing.T) {
	t.Run("explicit path returned as-is", func(t *testing.T) {
		got := ResolveConfigFilePath("/etc/mender-cli/.mender-clirc")
		if got != "/etc/mender-cli/.mender-clirc" {
			t.Errorf("got %q, want /etc/mender-cli/.mender-clirc", got)
		}
	})

	t.Run("empty falls back to home dir", func(t *testing.T) {
		got := ResolveConfigFilePath("")
		home, err := os.UserHomeDir()
		if err != nil {
			t.Skip("no home dir available")
		}
		want := filepath.Join(home, ".mender-clirc")
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}

func TestPersistServerToConfigReadError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".mender-clirc")
	os.Mkdir(path, 0700)

	_, err := PersistServerToConfig(path, "https://example.com")
	if err == nil {
		t.Fatal("expected error when path is a directory")
	}
}

func TestResolveConfigFilePathNoHome(t *testing.T) {
	got := ResolveConfigFilePath("")
	if got == "" {
		t.Error("expected non-empty path")
	}
}

func TestLoadConfigServerMalformedJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".mender-clirc")
	os.WriteFile(path, []byte(`{not json`), 0600)

	got := LoadConfigServer(path)
	if got != "" {
		t.Errorf("expected empty for malformed JSON, got %q", got)
	}
}

func TestLoadConfigServerNonStringValue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".mender-clirc")
	os.WriteFile(path, []byte(`{"server": 12345}`), 0600)

	got := LoadConfigServer(path)
	if got != "" {
		t.Errorf("expected empty for non-string server, got %q", got)
	}
}

func TestLoadConfigServer(t *testing.T) {
	t.Run("returns server from config", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, ".mender-clirc")
		os.WriteFile(path, []byte(`{"server": "https://my.server.io"}`), 0600)

		got := LoadConfigServer(path)
		if got != "https://my.server.io" {
			t.Errorf("got %q, want https://my.server.io", got)
		}
	})

	t.Run("returns empty for missing file", func(t *testing.T) {
		got := LoadConfigServer("/nonexistent/path/.mender-clirc")
		if got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})

	t.Run("returns empty for no server key", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, ".mender-clirc")
		os.WriteFile(path, []byte(`{"username": "alice"}`), 0600)

		got := LoadConfigServer(path)
		if got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})
}

func readJSONFile(t *testing.T, path string) map[string]interface{} {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file %s: %s", path, err)
	}
	var content map[string]interface{}
	if err := json.Unmarshal(data, &content); err != nil {
		t.Fatalf("failed to parse JSON file %s: %s", path, err)
	}
	return content
}
