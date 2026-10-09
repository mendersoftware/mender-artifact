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
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
)

const (
	loginURL            = "/api/management/v1/useradm/auth/login"
	artifactUploadURL   = "/api/management/v1/deployments/artifacts"
	artifactsListURL    = "/api/management/v1/deployments/artifacts/list"
	artifactURL         = "/api/management/v1/deployments/artifacts/:id"
	artifactDownloadURL = "/api/management/v1/deployments/artifacts/:id/download"
)

type ArtifactInfo struct {
	ID                    string   `json:"id"`
	Name                  string   `json:"name"`
	Description           string   `json:"description"`
	DeviceTypesCompatible []string `json:"device_types_compatible"`
	Size                  int64    `json:"size"`
	Modified              string   `json:"modified"`
	Info                  struct {
		Format  string `json:"format"`
		Version int    `json:"version"`
	} `json:"info"`
	Signed  bool `json:"signed"`
	Updates []struct {
		TypeInfo struct {
			Type string `json:"type"`
		} `json:"type_info"`
	} `json:"updates"`
}

type DownloadLink struct {
	URI    string    `json:"uri"`
	Expire time.Time `json:"expire"`
}

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(baseURL string, skipVerify bool) *Client {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: skipVerify}, //nolint:gosec
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Transport: tr,
		},
	}
}

func (c *Client) Login(user, pass, tfaToken string) (string, error) {
	url := c.baseURL + loginURL

	var reqBody io.Reader
	if tfaToken != "" {
		reqBody = strings.NewReader(`{"token2fa":"` + tfaToken + `"}`)
	}

	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodPost, url, reqBody,
	)
	if err != nil {
		return "", errors.Wrap(err, "failed to create login request")
	}
	req.SetBasicAuth(user, pass)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", errors.Wrap(err, "login request failed")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", errors.Wrap(err, "failed to read login response")
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("login failed with status %d: %s", resp.StatusCode, string(body))
	}

	return strings.TrimSpace(string(body)), nil
}

func (c *Client) UploadArtifact(artifactPath, token, description string) error {
	fi, err := os.Stat(artifactPath)
	if err != nil {
		return errors.Wrap(err, "failed to stat artifact file")
	}

	artifactFile, err := os.Open(artifactPath)
	if err != nil {
		return errors.Wrap(err, "failed to open artifact file")
	}
	defer artifactFile.Close()

	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)

	go func() {
		defer pw.Close()

		_ = writer.WriteField("size", strconv.FormatInt(fi.Size(), 10))
		if description != "" {
			_ = writer.WriteField("description", description)
		}

		part, err := writer.CreateFormFile("artifact", filepath.Base(artifactPath))
		if err != nil {
			pw.CloseWithError(err)
			return
		}
		if _, err = io.Copy(part, artifactFile); err != nil {
			pw.CloseWithError(err)
			return
		}
		writer.Close()
	}()

	url := c.baseURL + artifactUploadURL
	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodPost, url, pr,
	)
	if err != nil {
		return errors.Wrap(err, "failed to create upload request")
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return errors.Wrap(err, "upload request failed")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return errors.Wrap(err, "failed to read upload response")
	}

	switch resp.StatusCode {
	case http.StatusCreated:
		return nil
	case http.StatusUnauthorized:
		return fmt.Errorf("unauthorized: please run 'mender-artifact login' first")
	case http.StatusConflict:
		return fmt.Errorf("artifact already exists with same name/depends: %s", string(body))
	default:
		return fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(body))
	}
}

func (c *Client) ListArtifacts(token string, page, perPage int) ([]ArtifactInfo, error) {
	url := fmt.Sprintf("%s%s?page=%d&per_page=%d", c.baseURL, artifactsListURL, page, perPage)

	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodGet, url, nil,
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create list request")
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "list request failed")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read list response")
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list failed with status %d: %s", resp.StatusCode, string(body))
	}

	var artifacts []ArtifactInfo
	if err := json.Unmarshal(body, &artifacts); err != nil {
		return nil, errors.Wrap(err, "failed to parse artifact list")
	}

	return artifacts, nil
}

func (c *Client) GetArtifact(artifactID, token string) (*ArtifactInfo, error) {
	url := strings.Replace(c.baseURL+artifactURL, ":id", artifactID, 1)

	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodGet, url, nil,
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create get request")
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "get artifact request failed")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read artifact response")
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get artifact failed with status %d: %s",
			resp.StatusCode, string(body))
	}

	var artifact ArtifactInfo
	if err := json.Unmarshal(body, &artifact); err != nil {
		return nil, errors.Wrap(err, "failed to parse artifact info")
	}

	return &artifact, nil
}

func (c *Client) DeleteArtifact(artifactID, token string) error {
	url := strings.Replace(c.baseURL+artifactURL, ":id", artifactID, 1)

	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodDelete, url, nil,
	)
	if err != nil {
		return errors.Wrap(err, "failed to create delete request")
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return errors.Wrap(err, "delete artifact request failed")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return errors.Wrap(err, "failed to read delete response")
	}

	switch resp.StatusCode {
	case http.StatusNoContent:
		return nil
	case http.StatusUnauthorized:
		return fmt.Errorf("unauthorized: please run 'login' first")
	default:
		return fmt.Errorf("delete failed with status %d: %s", resp.StatusCode, string(body))
	}
}

func (c *Client) getDownloadLink(artifactID, token string) (*DownloadLink, error) {
	url := strings.Replace(c.baseURL+artifactDownloadURL, ":id", artifactID, 1)

	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodGet, url, nil,
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create download link request")
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "download link request failed")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read download link response")
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download link failed with status %d: %s",
			resp.StatusCode, string(body))
	}

	var link DownloadLink
	if err := json.Unmarshal(body, &link); err != nil {
		return nil, errors.Wrap(err, "failed to parse download link")
	}

	return &link, nil
}

func (c *Client) DownloadArtifact(artifactID, token, outputPath string) error {
	artifact, err := c.GetArtifact(artifactID, token)
	if err != nil {
		return err
	}

	link, err := c.getDownloadLink(artifactID, token)
	if err != nil {
		return err
	}

	if outputPath == "" {
		outputPath = artifact.Name + ".mender"
	}

	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodGet, link.URI, nil,
	)
	if err != nil {
		return errors.Wrap(err, "failed to create download request")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return errors.Wrap(err, "download request failed")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	f, err := os.Create(outputPath)
	if err != nil {
		return errors.Wrap(err, "failed to create output file")
	}
	defer f.Close()

	written, err := io.Copy(f, resp.Body)
	if err != nil {
		return errors.Wrap(err, "failed to write artifact file")
	}

	fmt.Fprintf(os.Stderr, "Downloaded %s (%d bytes) to %s\n", artifact.Name, written, outputPath)
	return nil
}

func (c *Client) FindArtifactsByName(name, token string) ([]ArtifactInfo, error) {
	var matches []ArtifactInfo
	page := 1
	for {
		artifacts, err := c.ListArtifacts(token, page, 100)
		if err != nil {
			return nil, err
		}
		if len(artifacts) == 0 {
			break
		}
		for i := range artifacts {
			if artifacts[i].Name == name {
				matches = append(matches, artifacts[i])
			}
		}
		page++
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("artifact %q not found", name)
	}
	return matches, nil
}

func (c *Client) FindArtifactByName(name, deviceType, token string) (*ArtifactInfo, error) {
	matches, err := c.FindArtifactsByName(name, token)
	if err != nil {
		return nil, err
	}

	if deviceType != "" {
		var filtered []ArtifactInfo
		for _, a := range matches {
			for _, dt := range a.DeviceTypesCompatible {
				if dt == deviceType {
					filtered = append(filtered, a)
					break
				}
			}
		}
		if len(filtered) == 0 {
			return nil, fmt.Errorf(
				"artifact %q found but none compatible with device type %q", name, deviceType)
		}
		matches = filtered
	}

	if len(matches) == 1 {
		return &matches[0], nil
	}

	var buf bytes.Buffer
	fmt.Fprintf(&buf, "artifact name %q matches %d artifacts (a release). ", name, len(matches))
	fmt.Fprintf(&buf, "Use --device-type to filter or specify the artifact ID directly:\n")
	for _, a := range matches {
		fmt.Fprintf(&buf, "  %s  device types: %s\n",
			a.ID, strings.Join(a.DeviceTypesCompatible, ", "))
	}
	return nil, fmt.Errorf("%s", buf.String())
}

func (c *Client) DownloadArtifactByName(name, deviceType, token, outputPath string) error {
	artifact, err := c.FindArtifactByName(name, deviceType, token)
	if err != nil {
		return err
	}
	return c.DownloadArtifact(artifact.ID, token, outputPath)
}

func (c *Client) DownloadArtifactToTempDir(artifactID, token, tmpDir string) (string, error) {
	artifact, err := c.GetArtifact(artifactID, token)
	if err != nil {
		return "", err
	}

	link, err := c.getDownloadLink(artifactID, token)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodGet, link.URI, nil,
	)
	if err != nil {
		return "", errors.Wrap(err, "failed to create download request")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", errors.Wrap(err, "download request failed")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	outputPath := filepath.Join(tmpDir, artifact.Name+".mender")
	f, err := os.Create(outputPath)
	if err != nil {
		return "", errors.Wrap(err, "failed to create temp file")
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return "", errors.Wrap(err, "failed to write artifact")
	}

	return outputPath, nil
}

func FormatArtifactList(artifacts []ArtifactInfo) string {
	if len(artifacts) == 0 {
		return "No artifacts found."
	}

	var buf bytes.Buffer
	fmt.Fprintf(&buf, "%-36s  %-30s  %-20s  %-10s  %s\n",
		"ID", "NAME", "TYPE", "SIZE", "MODIFIED")
	fmt.Fprintf(&buf, "%-36s  %-30s  %-20s  %-10s  %s\n",
		strings.Repeat("-", 36),
		strings.Repeat("-", 30),
		strings.Repeat("-", 20),
		strings.Repeat("-", 10),
		strings.Repeat("-", 20))

	for _, a := range artifacts {
		artType := ""
		if len(a.Updates) > 0 {
			artType = a.Updates[0].TypeInfo.Type
		}
		sizeStr := formatBytes(a.Size)
		modified := a.Modified
		if len(modified) > 19 {
			modified = modified[:19]
		}
		name := a.Name
		if len(name) > 30 {
			name = name[:27] + "..."
		}
		fmt.Fprintf(&buf, "%-36s  %-30s  %-20s  %-10s  %s\n",
			a.ID, name, artType, sizeStr, modified)
	}

	return buf.String()
}

func formatBytes(b int64) string {
	const unit = 1000
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "kMGTPE"[exp])
}
