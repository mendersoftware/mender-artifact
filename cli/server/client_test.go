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
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testServer(handler http.HandlerFunc) (*httptest.Server, *Client) {
	ts := httptest.NewServer(handler)
	client := NewClient(ts.URL, false)
	return ts, client
}

func TestLogin(t *testing.T) {
	ts, client := testServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != loginURL {
			http.NotFound(w, r)
			return
		}
		user, pass, ok := r.BasicAuth()
		if !ok || user != "admin" || pass != "secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		fmt.Fprint(w, "jwt-token-value")
	})
	defer ts.Close()

	token, err := client.Login("admin", "secret", "")
	if err != nil {
		t.Fatalf("Login failed: %s", err)
	}
	if token != "jwt-token-value" {
		t.Errorf("got %q, want jwt-token-value", token)
	}
}

func TestLoginWith2FA(t *testing.T) {
	ts, client := testServer(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"token2fa":"123456"`) {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, "2fa required")
			return
		}
		fmt.Fprint(w, "jwt-2fa-token")
	})
	defer ts.Close()

	token, err := client.Login("admin", "secret", "123456")
	if err != nil {
		t.Fatalf("Login failed: %s", err)
	}
	if token != "jwt-2fa-token" {
		t.Errorf("got %q, want jwt-2fa-token", token)
	}
}

func TestLoginFailure(t *testing.T) {
	ts, client := testServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, "bad credentials")
	})
	defer ts.Close()

	_, err := client.Login("wrong", "creds", "")
	if err == nil {
		t.Fatal("expected error for failed login")
	}
}

func TestUploadArtifact(t *testing.T) {
	ts, client := testServer(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if !strings.HasPrefix(
			r.Header.Get("Content-Type"), "multipart/form-data",
		) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusCreated)
	})
	defer ts.Close()

	tmpFile := filepath.Join(t.TempDir(), "test.mender")
	os.WriteFile(tmpFile, []byte("artifact-data"), 0600)

	err := client.UploadArtifact(tmpFile, "token", "test upload")
	if err != nil {
		t.Fatalf("UploadArtifact failed: %s", err)
	}
}

func TestUploadArtifactConflict(t *testing.T) {
	ts, client := testServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		fmt.Fprint(w, "duplicate")
	})
	defer ts.Close()

	tmpFile := filepath.Join(t.TempDir(), "test.mender")
	os.WriteFile(tmpFile, []byte("data"), 0600)

	err := client.UploadArtifact(tmpFile, "token", "")
	if err == nil {
		t.Fatal("expected error for conflict")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("unexpected error: %s", err)
	}
}

func TestUploadArtifactUnauthorized(t *testing.T) {
	ts, client := testServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	defer ts.Close()

	tmpFile := filepath.Join(t.TempDir(), "test.mender")
	os.WriteFile(tmpFile, []byte("data"), 0600)

	err := client.UploadArtifact(tmpFile, "bad-token", "")
	if err == nil {
		t.Fatal("expected error for unauthorized")
	}
	if !strings.Contains(err.Error(), "unauthorized") {
		t.Errorf("unexpected error: %s", err)
	}
}

func TestListArtifacts(t *testing.T) {
	artifacts := []ArtifactInfo{
		{ID: "id-1", Name: "art-1", Size: 1024},
		{ID: "id-2", Name: "art-2", Size: 2048},
	}
	ts, client := testServer(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(artifacts)
	})
	defer ts.Close()

	result, err := client.ListArtifacts("token", 1, 20)
	if err != nil {
		t.Fatalf("ListArtifacts failed: %s", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 artifacts, got %d", len(result))
	}
	if result[0].Name != "art-1" {
		t.Errorf("got %q, want art-1", result[0].Name)
	}
}

func TestListArtifactsError(t *testing.T) {
	ts, client := testServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "internal error")
	})
	defer ts.Close()

	_, err := client.ListArtifacts("token", 1, 20)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGetArtifact(t *testing.T) {
	artifact := ArtifactInfo{
		ID: "abc-123", Name: "my-artifact", Size: 4096,
	}
	ts, client := testServer(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "abc-123") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(artifact)
	})
	defer ts.Close()

	result, err := client.GetArtifact("abc-123", "token")
	if err != nil {
		t.Fatalf("GetArtifact failed: %s", err)
	}
	if result.Name != "my-artifact" {
		t.Errorf("got %q, want my-artifact", result.Name)
	}
}

func TestGetArtifactNotFound(t *testing.T) {
	ts, client := testServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, "not found")
	})
	defer ts.Close()

	_, err := client.GetArtifact("missing-id", "token")
	if err == nil {
		t.Fatal("expected error for not found")
	}
}

func TestDeleteArtifact(t *testing.T) {
	ts, client := testServer(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	defer ts.Close()

	err := client.DeleteArtifact("abc-123", "token")
	if err != nil {
		t.Fatalf("DeleteArtifact failed: %s", err)
	}
}

func TestDeleteArtifactUnauthorized(t *testing.T) {
	ts, client := testServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	defer ts.Close()

	err := client.DeleteArtifact("abc-123", "bad-token")
	if err == nil {
		t.Fatal("expected error for unauthorized")
	}
}

func TestDownloadArtifact(t *testing.T) {
	ts, client := testServer(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/download") {
			link := DownloadLink{URI: "http://" + r.Host + "/file"}
			json.NewEncoder(w).Encode(link)
			return
		}
		if r.URL.Path == "/file" {
			w.Write([]byte("artifact-content"))
			return
		}
		artifact := ArtifactInfo{ID: "id-1", Name: "test-art"}
		json.NewEncoder(w).Encode(artifact)
	})
	defer ts.Close()

	outputPath := filepath.Join(t.TempDir(), "downloaded.mender")
	err := client.DownloadArtifact("id-1", "token", outputPath)
	if err != nil {
		t.Fatalf("DownloadArtifact failed: %s", err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read downloaded file: %s", err)
	}
	if string(data) != "artifact-content" {
		t.Errorf("got %q, want artifact-content", string(data))
	}
}

func TestDownloadArtifactDefaultName(t *testing.T) {
	ts, client := testServer(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/download") {
			link := DownloadLink{URI: "http://" + r.Host + "/file"}
			json.NewEncoder(w).Encode(link)
			return
		}
		if r.URL.Path == "/file" {
			w.Write([]byte("data"))
			return
		}
		artifact := ArtifactInfo{ID: "id-1", Name: "my-art"}
		json.NewEncoder(w).Encode(artifact)
	})
	defer ts.Close()

	origDir, _ := os.Getwd()
	tmpDir := t.TempDir()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	err := client.DownloadArtifact("id-1", "token", "")
	if err != nil {
		t.Fatalf("DownloadArtifact failed: %s", err)
	}

	if _, err := os.Stat(filepath.Join(tmpDir, "my-art.mender")); err != nil {
		t.Errorf("expected default-named file: %s", err)
	}
}

func TestDownloadArtifactToTempDir(t *testing.T) {
	ts, client := testServer(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/download") {
			link := DownloadLink{URI: "http://" + r.Host + "/file"}
			json.NewEncoder(w).Encode(link)
			return
		}
		if r.URL.Path == "/file" {
			w.Write([]byte("temp-content"))
			return
		}
		artifact := ArtifactInfo{ID: "id-1", Name: "tmp-art"}
		json.NewEncoder(w).Encode(artifact)
	})
	defer ts.Close()

	tmpDir := t.TempDir()
	path, err := client.DownloadArtifactToTempDir("id-1", "token", tmpDir)
	if err != nil {
		t.Fatalf("DownloadArtifactToTempDir failed: %s", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %s", err)
	}
	if string(data) != "temp-content" {
		t.Errorf("got %q, want temp-content", string(data))
	}
}

// paginatedHandler returns artifacts on page 1, empty on subsequent pages.
func paginatedHandler(artifacts []ArtifactInfo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		if page != "" && page != "1" {
			json.NewEncoder(w).Encode([]ArtifactInfo{})
			return
		}
		json.NewEncoder(w).Encode(artifacts)
	}
}

func TestDownloadArtifactToTempDirError(t *testing.T) {
	ts, client := testServer(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/download") {
			link := DownloadLink{URI: "http://" + r.Host + "/file"}
			json.NewEncoder(w).Encode(link)
			return
		}
		if r.URL.Path == "/file" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		artifact := ArtifactInfo{ID: "id-1", Name: "err-art"}
		json.NewEncoder(w).Encode(artifact)
	})
	defer ts.Close()

	_, err := client.DownloadArtifactToTempDir(
		"id-1", "token", t.TempDir(),
	)
	if err == nil {
		t.Fatal("expected error for failed download")
	}
}

func TestDownloadArtifactNotFound(t *testing.T) {
	ts, client := testServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, "not found")
	})
	defer ts.Close()

	err := client.DownloadArtifact(
		"missing-id", "token", filepath.Join(t.TempDir(), "out"),
	)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDeleteArtifactServerError(t *testing.T) {
	ts, client := testServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "server error")
	})
	defer ts.Close()

	err := client.DeleteArtifact("id-1", "token")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("unexpected error: %s", err)
	}
}

func TestUploadArtifactServerError(t *testing.T) {
	ts, client := testServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "error")
	})
	defer ts.Close()

	tmpFile := filepath.Join(t.TempDir(), "test.mender")
	os.WriteFile(tmpFile, []byte("data"), 0600)

	err := client.UploadArtifact(tmpFile, "token", "")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestFindArtifactsByName(t *testing.T) {
	allArtifacts := []ArtifactInfo{
		{
			ID: "id-1", Name: "release-v1",
			DeviceTypesCompatible: []string{"rpi4"},
		},
		{
			ID: "id-2", Name: "release-v1",
			DeviceTypesCompatible: []string{"beaglebone"},
		},
		{ID: "id-3", Name: "other"},
	}
	ts, client := testServer(paginatedHandler(allArtifacts))
	defer ts.Close()

	matches, err := client.FindArtifactsByName("release-v1", "token")
	if err != nil {
		t.Fatalf("FindArtifactsByName failed: %s", err)
	}
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}
}

func TestFindArtifactsByNameNotFound(t *testing.T) {
	ts, client := testServer(paginatedHandler([]ArtifactInfo{}))
	defer ts.Close()

	_, err := client.FindArtifactsByName("nonexistent", "token")
	if err == nil {
		t.Fatal("expected error for not found")
	}
}

func TestFindArtifactByNameUnique(t *testing.T) {
	allArtifacts := []ArtifactInfo{
		{
			ID: "id-1", Name: "unique-art",
			DeviceTypesCompatible: []string{"rpi4"},
		},
		{ID: "id-2", Name: "other"},
	}
	ts, client := testServer(paginatedHandler(allArtifacts))
	defer ts.Close()

	art, err := client.FindArtifactByName("unique-art", "", "token")
	if err != nil {
		t.Fatalf("FindArtifactByName failed: %s", err)
	}
	if art.ID != "id-1" {
		t.Errorf("got ID %q, want id-1", art.ID)
	}
}

func TestFindArtifactByNameAmbiguous(t *testing.T) {
	allArtifacts := []ArtifactInfo{
		{
			ID: "id-1", Name: "release",
			DeviceTypesCompatible: []string{"rpi4"},
		},
		{
			ID: "id-2", Name: "release",
			DeviceTypesCompatible: []string{"beaglebone"},
		},
	}
	ts, client := testServer(paginatedHandler(allArtifacts))
	defer ts.Close()

	_, err := client.FindArtifactByName("release", "", "token")
	if err == nil {
		t.Fatal("expected error for ambiguous name")
	}
	if !strings.Contains(err.Error(), "matches 2 artifacts") {
		t.Errorf("unexpected error: %s", err)
	}
}

func TestFindArtifactByNameWithDeviceType(t *testing.T) {
	allArtifacts := []ArtifactInfo{
		{
			ID: "id-1", Name: "release",
			DeviceTypesCompatible: []string{"rpi4"},
		},
		{
			ID: "id-2", Name: "release",
			DeviceTypesCompatible: []string{"beaglebone"},
		},
	}
	ts, client := testServer(paginatedHandler(allArtifacts))
	defer ts.Close()

	art, err := client.FindArtifactByName(
		"release", "beaglebone", "token",
	)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if art.ID != "id-2" {
		t.Errorf("got ID %q, want id-2", art.ID)
	}
}

func TestFindArtifactByNameDeviceTypeNoMatch(t *testing.T) {
	allArtifacts := []ArtifactInfo{
		{
			ID: "id-1", Name: "release",
			DeviceTypesCompatible: []string{"rpi4"},
		},
	}
	ts, client := testServer(paginatedHandler(allArtifacts))
	defer ts.Close()

	_, err := client.FindArtifactByName(
		"release", "unknown-dt", "token",
	)
	if err == nil {
		t.Fatal("expected error for no device type match")
	}
	if !strings.Contains(err.Error(), "none compatible") {
		t.Errorf("unexpected error: %s", err)
	}
}

func TestDownloadArtifactByName(t *testing.T) {
	allArtifacts := []ArtifactInfo{
		{ID: "id-1", Name: "my-art", Size: 100},
	}
	ts, client := testServer(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/list") {
			page := r.URL.Query().Get("page")
			if page != "" && page != "1" {
				json.NewEncoder(w).Encode([]ArtifactInfo{})
				return
			}
			json.NewEncoder(w).Encode(allArtifacts)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/download") {
			link := DownloadLink{URI: "http://" + r.Host + "/file"}
			json.NewEncoder(w).Encode(link)
			return
		}
		if r.URL.Path == "/file" {
			w.Write([]byte("by-name-content"))
			return
		}
		json.NewEncoder(w).Encode(allArtifacts[0])
	})
	defer ts.Close()

	outputPath := filepath.Join(t.TempDir(), "out.mender")
	err := client.DownloadArtifactByName(
		"my-art", "", "token", outputPath,
	)
	if err != nil {
		t.Fatalf("DownloadArtifactByName failed: %s", err)
	}

	data, _ := os.ReadFile(outputPath)
	if string(data) != "by-name-content" {
		t.Errorf("got %q, want by-name-content", string(data))
	}
}

func TestFormatArtifactListEmpty(t *testing.T) {
	result := FormatArtifactList(nil)
	if result != "No artifacts found." {
		t.Errorf("got %q", result)
	}
}

func TestFormatArtifactList(t *testing.T) {
	artifacts := []ArtifactInfo{
		{
			ID: "abc-123", Name: "test", Size: 1500,
			Modified: "2026-01-01T00:00:00Z",
		},
	}
	result := FormatArtifactList(artifacts)
	if !strings.Contains(result, "abc-123") {
		t.Errorf("expected ID in output: %s", result)
	}
	if !strings.Contains(result, "test") {
		t.Errorf("expected name in output: %s", result)
	}
}

func TestUploadArtifactMissingFile(t *testing.T) {
	ts, client := testServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	defer ts.Close()

	err := client.UploadArtifact("/nonexistent/file.mender", "token", "")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestUploadArtifactWithDescription(t *testing.T) {
	var gotDesc bool
	ts, client := testServer(func(w http.ResponseWriter, r *http.Request) {
		err := r.ParseMultipartForm(10 << 20)
		if err == nil && r.FormValue("description") == "my desc" {
			gotDesc = true
		}
		w.WriteHeader(http.StatusCreated)
	})
	defer ts.Close()

	tmpFile := filepath.Join(t.TempDir(), "test.mender")
	os.WriteFile(tmpFile, []byte("data"), 0600)

	err := client.UploadArtifact(tmpFile, "token", "my desc")
	if err != nil {
		t.Fatalf("UploadArtifact failed: %s", err)
	}
	if !gotDesc {
		t.Error("description field not received by server")
	}
}

func TestListArtifactsInvalidJSON(t *testing.T) {
	ts, client := testServer(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "not-json")
	})
	defer ts.Close()

	_, err := client.ListArtifacts("token", 1, 20)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestGetArtifactInvalidJSON(t *testing.T) {
	ts, client := testServer(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "not-json")
	})
	defer ts.Close()

	_, err := client.GetArtifact("id-1", "token")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestDownloadArtifactLinkError(t *testing.T) {
	callCount := 0
	ts, client := testServer(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			json.NewEncoder(w).Encode(ArtifactInfo{
				ID: "id-1", Name: "art",
			})
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "link error")
	})
	defer ts.Close()

	err := client.DownloadArtifact(
		"id-1", "token", filepath.Join(t.TempDir(), "out"),
	)
	if err == nil {
		t.Fatal("expected error when download link fails")
	}
}

func TestDownloadArtifactHTTPError(t *testing.T) {
	ts, client := testServer(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/download") {
			link := DownloadLink{URI: "http://" + r.Host + "/file"}
			json.NewEncoder(w).Encode(link)
			return
		}
		if r.URL.Path == "/file" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		json.NewEncoder(w).Encode(ArtifactInfo{
			ID: "id-1", Name: "art",
		})
	})
	defer ts.Close()

	err := client.DownloadArtifact(
		"id-1", "token", filepath.Join(t.TempDir(), "out"),
	)
	if err == nil {
		t.Fatal("expected error for forbidden download")
	}
}

func TestDownloadLinkInvalidJSON(t *testing.T) {
	callCount := 0
	ts, client := testServer(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			json.NewEncoder(w).Encode(ArtifactInfo{
				ID: "id-1", Name: "art",
			})
			return
		}
		fmt.Fprint(w, "not-json")
	})
	defer ts.Close()

	err := client.DownloadArtifact(
		"id-1", "token", filepath.Join(t.TempDir(), "out"),
	)
	if err == nil {
		t.Fatal("expected error for invalid link JSON")
	}
}

func TestDownloadArtifactByNameError(t *testing.T) {
	ts, client := testServer(paginatedHandler([]ArtifactInfo{}))
	defer ts.Close()

	err := client.DownloadArtifactByName(
		"missing", "", "token", filepath.Join(t.TempDir(), "out"),
	)
	if err == nil {
		t.Fatal("expected error for missing artifact")
	}
}

func TestDownloadArtifactToTempDirGetError(t *testing.T) {
	ts, client := testServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, "not found")
	})
	defer ts.Close()

	_, err := client.DownloadArtifactToTempDir(
		"missing-id", "token", t.TempDir(),
	)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDownloadArtifactToTempDirLinkError(t *testing.T) {
	callCount := 0
	ts, client := testServer(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			json.NewEncoder(w).Encode(ArtifactInfo{
				ID: "id-1", Name: "art",
			})
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	})
	defer ts.Close()

	_, err := client.DownloadArtifactToTempDir(
		"id-1", "token", t.TempDir(),
	)
	if err == nil {
		t.Fatal("expected error when link fails")
	}
}

func TestFormatArtifactListLongName(t *testing.T) {
	artifacts := []ArtifactInfo{
		{
			ID:       "id-1",
			Name:     "this-is-a-very-long-artifact-name-that-exceeds-thirty-chars",
			Size:     999,
			Modified: "2026-01-01T00:00:00.000000Z",
		},
	}
	result := FormatArtifactList(artifacts)
	if !strings.Contains(result, "...") {
		t.Error("expected long name to be truncated with ...")
	}
	if !strings.Contains(result, "2026-01-01T00:00:00") {
		t.Error("expected modified date to be truncated to 19 chars")
	}
}

func TestFormatArtifactListWithType(t *testing.T) {
	artifacts := []ArtifactInfo{
		{
			ID: "id-1", Name: "typed", Size: 5000,
			Updates: []struct {
				TypeInfo struct {
					Type string `json:"type"`
				} `json:"type_info"`
			}{
				{TypeInfo: struct {
					Type string `json:"type"`
				}{Type: "rootfs-image"}},
			},
		},
	}
	result := FormatArtifactList(artifacts)
	if !strings.Contains(result, "rootfs-image") {
		t.Errorf("expected type in output: %s", result)
	}
}

func TestFindArtifactsByNameListError(t *testing.T) {
	ts, client := testServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	defer ts.Close()

	_, err := client.FindArtifactsByName("any", "token")
	if err == nil {
		t.Fatal("expected error when list fails")
	}
}

func TestFormatBytes(t *testing.T) {
	cases := []struct {
		input int64
		want  string
	}{
		{0, "0 B"},
		{500, "500 B"},
		{1500, "1.5 kB"},
		{1500000, "1.5 MB"},
		{1500000000, "1.5 GB"},
	}
	for _, tc := range cases {
		got := formatBytes(tc.input)
		if got != tc.want {
			t.Errorf("formatBytes(%d) = %q, want %q",
				tc.input, got, tc.want)
		}
	}
}
