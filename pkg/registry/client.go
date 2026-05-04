package registry

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/OpceanAI/Doki/pkg/common"
)

const (
	DefaultRegistry = "registry-1.docker.io"
	DefaultTag      = "latest"
	AuthService     = "registry.docker.io"
	AuthRealm       = "https://auth.docker.io/token"
)

type Client struct {
	httpClient  *http.Client
	userAgent   string
	tokens      map[string]*tokenCache
	mu          sync.RWMutex
}

type tokenCache struct {
	token     string
	expiresAt time.Time
}

type AuthConfig struct {
	Username      string `json:"username,omitempty"`
	Password      string `json:"password,omitempty"`
	ServerAddress string `json:"serveraddress,omitempty"`
	IdentityToken string `json:"identitytoken,omitempty"`
}

func NewClient(insecure bool) *Client {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: insecure,
		},
	}

	return &Client{
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   120 * time.Second,
		},
		userAgent: common.UserAgent(),
		tokens:    make(map[string]*tokenCache),
	}
}

type ImageRef struct {
	Registry string
	Name     string
	Tag      string
	Digest   string
}

func ParseImageRef(ref string) (*ImageRef, error) {
	ir := &ImageRef{Tag: DefaultTag}

	if idx := strings.Index(ref, "@"); idx != -1 {
		ir.Digest = ref[idx+1:]
		ref = ref[:idx]
	}
	if idx := strings.Index(ref, ":"); idx != -1 {
		ir.Tag = ref[idx+1:]
		ref = ref[:idx]
	}

	parts := strings.SplitN(ref, "/", 3)
	switch len(parts) {
	case 1:
		ir.Registry = DefaultRegistry
		ir.Name = "library/" + parts[0]
	case 2:
		if strings.Contains(parts[0], ".") || parts[0] == "localhost" {
			ir.Registry = parts[0]
			ir.Name = parts[1]
		} else {
			ir.Registry = DefaultRegistry
			ir.Name = parts[0] + "/" + parts[1]
		}
	case 3:
		ir.Registry = parts[0]
		ir.Name = parts[1] + "/" + parts[2]
	}

	return ir, nil
}

func (ir *ImageRef) String() string {
	base := fmt.Sprintf("%s/%s", ir.Registry, ir.Name)
	if ir.Digest != "" {
		return base + "@" + ir.Digest
	}
	return base + ":" + ir.Tag
}

func (ir *ImageRef) FullName() string {
	return fmt.Sprintf("%s/%s", ir.Registry, ir.Name)
}

type ManifestV2 struct {
	SchemaVersion int               `json:"schemaVersion"`
	MediaType     string            `json:"mediaType"`
	Config        ManifestBlob      `json:"config"`
	Layers        []ManifestBlob    `json:"layers"`
	Annotations   map[string]string `json:"annotations,omitempty"`
}

type ManifestBlob struct {
	MediaType   string            `json:"mediaType"`
	Size        int64             `json:"size"`
	Digest      string            `json:"digest"`
	URLs        []string          `json:"urls,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

type ManifestList struct {
	SchemaVersion int               `json:"schemaVersion"`
	MediaType     string            `json:"mediaType"`
	Manifests     []ManifestListEntry `json:"manifests"`
}

type ManifestListEntry struct {
	MediaType   string            `json:"mediaType"`
	Size        int64             `json:"size"`
	Digest      string            `json:"digest"`
	Platform    *Platform          `json:"platform"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

type Platform struct {
	Architecture string `json:"architecture"`
	OS           string `json:"os"`
	Variant      string `json:"variant,omitempty"`
}

type TagList struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

// getToken gets or fetches an auth token for a registry+scope.
func (c *Client) getToken(registry, service, scope string) (string, error) {
	key := registry + "|" + scope
	c.mu.RLock()
	if tc, ok := c.tokens[key]; ok && time.Now().Before(tc.expiresAt) {
		c.mu.RUnlock()
		return tc.token, nil
	}
	c.mu.RUnlock()

	realm := AuthRealm
	if service == "" {
		service = AuthService
	}

	tokenURL, _ := url.Parse(realm)
	q := tokenURL.Query()
	q.Set("service", service)
	q.Set("scope", scope)
	tokenURL.RawQuery = q.Encode()

	req, _ := http.NewRequest("GET", tokenURL.String(), nil)
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("token request failed (status %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		IssuedAt    string `json:"issued_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode token response: %w", err)
	}

	token := result.Token
	if token == "" {
		token = result.AccessToken
	}
	if token == "" {
		return "", fmt.Errorf("no token in response")
	}

	expiresIn := result.ExpiresIn
	if expiresIn < 60 {
		expiresIn = 300
	}

	c.mu.Lock()
	c.tokens[key] = &tokenCache{
		token:     token,
		expiresAt: time.Now().Add(time.Duration(expiresIn) * time.Second),
	}
	c.mu.Unlock()

	return token, nil
}

// doAuthRequest performs an authenticated request with automatic token retry.
func (c *Client) doAuthRequest(method, urlStr string, headers map[string]string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, urlStr, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusUnauthorized {
		authHeader := resp.Header.Get("Www-Authenticate")
		if authHeader == "" {
			authHeader = resp.Header.Get("WWW-Authenticate")
		}

		if authHeader != "" {
			resp.Body.Close()
			realm, service, scope := parseWwwAuthenticate(authHeader)
			token, err := c.getToken(realm, service, scope)
			if err != nil {
				return nil, fmt.Errorf("get token: %w", err)
			}

			req2, _ := http.NewRequest(method, urlStr, body)
			req2.Header.Set("User-Agent", c.userAgent)
			req2.Header.Set("Authorization", "Bearer "+token)
			for k, v := range headers {
				req2.Header.Set(k, v)
			}

			return c.httpClient.Do(req2)
		}
	}

	return resp, nil
}

func parseWwwAuthenticate(header string) (realm, service, scope string) {
	header = strings.TrimPrefix(header, "Bearer ")
	parts := strings.Split(header, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		val := strings.Trim(kv[1], `"`)
		switch kv[0] {
		case "realm":
			realm = val
		case "service":
			service = val
		case "scope":
			scope = val
		}
	}
	return
}

func (c *Client) Ping(registry string) error {
	u := fmt.Sprintf("https://%s/v2/", registry)
	resp, err := c.doAuthRequest("GET", u, nil, nil)
	if err != nil {
		return fmt.Errorf("ping %s: %w", registry, err)
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusUnauthorized {
		return nil
	}
	return fmt.Errorf("registry returned %d", resp.StatusCode)
}

func (c *Client) GetTags(registry, repository string) (*TagList, error) {
	u := fmt.Sprintf("https://%s/v2/%s/tags/list", registry, repository)
	resp, err := c.doAuthRequest("GET", u, nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list tags: status %d", resp.StatusCode)
	}
	var tags TagList
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		return nil, err
	}
	return &tags, nil
}

func (c *Client) GetManifest(registry, name, reference string) (*ManifestV2, string, error) {
	accept := strings.Join([]string{
		"application/vnd.oci.image.manifest.v1+json",
		"application/vnd.docker.distribution.manifest.v2+json",
		"application/vnd.oci.image.index.v1+json",
		"application/vnd.docker.distribution.manifest.list.v2+json",
	}, ", ")

	u := fmt.Sprintf("https://%s/v2/%s/manifests/%s", registry, name, reference)
	headers := map[string]string{"Accept": accept}
	resp, err := c.doAuthRequest("GET", u, headers, nil)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, "", fmt.Errorf("get manifest %s/%s:%s: status %d body=%s", registry, name, reference, resp.StatusCode, string(body))
	}

	contentType := resp.Header.Get("Content-Type")
	digest := resp.Header.Get("Docker-Content-Digest")

	var manifest ManifestV2
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return nil, "", err
	}
	return &manifest, contentType + "|" + digest, nil
}

func (c *Client) DownloadBlob(registry, name, digest string, writer io.Writer) error {
	u := fmt.Sprintf("https://%s/v2/%s/blobs/%s", registry, name, digest)
	resp, err := c.doAuthRequest("GET", u, nil, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("download blob %s: status %d: %s", digest, resp.StatusCode, string(body))
	}
	_, err = io.Copy(writer, resp.Body)
	return err
}

func (c *Client) HeadBlob(registry, name, digest string) (int64, error) {
	u := fmt.Sprintf("https://%s/v2/%s/blobs/%s", registry, name, digest)
	resp, err := c.doAuthRequest("HEAD", u, nil, nil)
	if err != nil {
		return 0, err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("blob %s not found: %d", digest, resp.StatusCode)
	}
	return resp.ContentLength, nil
}

func (c *Client) GetBlob(registry, name, digest string) ([]byte, error) {
	u := fmt.Sprintf("https://%s/v2/%s/blobs/%s", registry, name, digest)
	resp, err := c.doAuthRequest("GET", u, nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get blob %s: %d: %s", digest, resp.StatusCode, string(body))
	}
	return io.ReadAll(resp.Body)
}

func (c *Client) DoRequest(ctx context.Context, method, urlStr string, headers map[string]string, body io.Reader) (*http.Response, error) {
	return c.doAuthRequest(method, urlStr, headers, body)
}

func (c *Client) GetConfig(registry, name string, manifest *ManifestV2) ([]byte, error) {
	return c.GetBlob(registry, name, manifest.Config.Digest)
}

func (c *Client) Close() {
	c.httpClient.CloseIdleConnections()
}

// ResolveManifest resolves a manifest for the current platform (linux/arm64 preferred).
func (c *Client) ResolveManifest(registry, name, reference string) (*ManifestV2, string, error) {
	accept := strings.Join([]string{
		"application/vnd.oci.image.manifest.v1+json",
		"application/vnd.docker.distribution.manifest.v2+json",
		"application/vnd.oci.image.index.v1+json",
		"application/vnd.docker.distribution.manifest.list.v2+json",
	}, ", ")

	u := fmt.Sprintf("https://%s/v2/%s/manifests/%s", registry, name, reference)
	headers := map[string]string{"Accept": accept}
	resp, err := c.doAuthRequest("GET", u, headers, nil)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, "", fmt.Errorf("resolve manifest %s/%s:%s: %d body=%s", registry, name, reference, resp.StatusCode, string(body))
	}

	contentType := resp.Header.Get("Content-Type")
	digest := resp.Header.Get("Docker-Content-Digest")

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}

	// Check if it's a manifest list (multi-arch).
	if strings.Contains(contentType, "manifest.list") ||
		strings.Contains(contentType, "index") ||
		strings.Contains(string(body), `"manifests":`) {

		var list ManifestList
		if err := json.Unmarshal(body, &list); err != nil {
			return nil, "", err
		}

		// Find best matching platform.
		// Prefer arm64 on ARM systems, amd64 on x86 systems.
		currentArch := runtime.GOARCH
		var bestMatch *ManifestListEntry

		for i := range list.Manifests {
			entry := &list.Manifests[i]
			if entry.Platform == nil || entry.Platform.OS != "linux" {
				continue
			}
			if entry.Platform.Architecture == currentArch {
				bestMatch = entry
				break
			}
			if entry.Platform.Architecture == "amd64" && bestMatch == nil {
				bestMatch = entry
			}
		}

		if bestMatch != nil {
			return c.GetManifest(registry, name, bestMatch.Digest)
		}
		// Fallback to first entry.
		if len(list.Manifests) > 0 {
			return c.GetManifest(registry, name, list.Manifests[0].Digest)
		}
		return nil, "", fmt.Errorf("no matching platform in manifest list")
	}

	var manifest ManifestV2
	if err := json.Unmarshal(body, &manifest); err != nil {
		return nil, "", err
	}
	return &manifest, contentType + "|" + digest, nil
}
