package registry

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
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
	basicUser   string
	basicPass   string
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

func (c *Client) SetAuth(username, password string) {
	c.basicUser = username
	c.basicPass = password
}

func NewClient(insecure bool) *Client {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: insecure,
		},
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 20,
		MaxConnsPerHost:     50,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
		DisableCompression:  false,
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
func (c *Client) getToken(realm, service, scope string) (string, error) {
	key := realm + "|" + scope
	c.mu.RLock()
	if tc, ok := c.tokens[key]; ok && time.Now().Before(tc.expiresAt) {
		c.mu.RUnlock()
		return tc.token, nil
	}
	c.mu.RUnlock()

	if realm == "" {
		realm = AuthRealm
	}
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

	if c.basicUser != "" && c.basicPass != "" {
		auth := base64.StdEncoding.EncodeToString([]byte(c.basicUser + ":" + c.basicPass))
		req.Header.Set("Authorization", "Basic "+auth)
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

	hash := sha256.New()
	tee := io.TeeReader(resp.Body, hash)
	_, err = io.Copy(writer, tee)
	if err != nil {
		return err
	}

	computedDigest := fmt.Sprintf("sha256:%x", hash.Sum(nil))

	if respDigest := resp.Header.Get("Docker-Content-Digest"); respDigest != "" && respDigest != digest {
		return fmt.Errorf("blob digest header mismatch: expected %s, got %s", digest, respDigest)
	}

	if computedDigest != digest {
		return fmt.Errorf("blob digest mismatch: expected %s, got %s", digest, computedDigest)
	}

	return nil
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

func (c *Client) Push(registry, name, tag string, manifest *ManifestV2, config []byte, layers map[string]io.Reader) error {
	configDigest := fmt.Sprintf("sha256:%x", sha256.Sum256(config))

	commonSources := []string{
		"library/alpine", "library/ubuntu", "library/debian",
		"library/centos", "library/fedora", "library/archlinux",
	}

	blobExists := func(digest string) bool {
		u := fmt.Sprintf("https://%s/v2/%s/blobs/%s", registry, name, digest)
		resp, err := c.doAuthRequest("HEAD", u, nil, nil)
		if err != nil {
			return false
		}
		resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}

	tryCrossMount := func(digest, sourceRepo string) bool {
		mountURL := fmt.Sprintf("https://%s/v2/%s/blobs/uploads/?mount=%s&from=%s",
			registry, name, digest, sourceRepo)
		resp, err := c.doAuthRequest("POST", mountURL, nil, nil)
		if err != nil {
			return false
		}
		resp.Body.Close()
		return resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusAccepted
	}

	uploadBlob := func(digest string, data []byte) error {
		initURL := fmt.Sprintf("https://%s/v2/%s/blobs/uploads/", registry, name)
		resp, err := c.doAuthRequest("POST", initURL, nil, nil)
		if err != nil {
			return fmt.Errorf("initiate upload: %w", err)
		}
		location := resp.Header.Get("Location")
		resp.Body.Close()
		if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusCreated {
			return fmt.Errorf("initiate upload: status %d", resp.StatusCode)
		}
		if location == "" {
			return fmt.Errorf("no upload location returned")
		}

		finalURL := fmt.Sprintf("%s&digest=%s", location, url.QueryEscape(digest))
		uploadResp, err := c.doAuthRequest("PUT", finalURL, map[string]string{
			"Content-Type":   "application/octet-stream",
			"Content-Length": fmt.Sprintf("%d", len(data)),
		}, bytes.NewReader(data))
		if err != nil {
			return fmt.Errorf("upload blob: %w", err)
		}
		uploadResp.Body.Close()
		if uploadResp.StatusCode != http.StatusCreated {
			return fmt.Errorf("upload blob: status %d", uploadResp.StatusCode)
		}
		return nil
	}

	// upload config blob
	if !blobExists(configDigest) {
		mounted := false
		for _, src := range commonSources {
			if tryCrossMount(configDigest, src) {
				mounted = true
				break
			}
		}
		if !mounted {
			if err := uploadBlob(configDigest, config); err != nil {
				return fmt.Errorf("upload config: %w", err)
			}
		}
	}

	// upload layer blobs
	for _, layer := range manifest.Layers {
		if !blobExists(layer.Digest) {
			mounted := false
			for _, src := range commonSources {
				if tryCrossMount(layer.Digest, src) {
					mounted = true
					break
				}
			}
			if !mounted {
				reader, ok := layers[layer.Digest]
				if !ok {
					return fmt.Errorf("layer data not provided for %s", layer.Digest)
				}
				layerData, err := io.ReadAll(reader)
				if err != nil {
					return fmt.Errorf("read layer %s: %w", layer.Digest, err)
				}
				expectedDigest := fmt.Sprintf("sha256:%x", sha256.Sum256(layerData))
				if expectedDigest != layer.Digest {
					return fmt.Errorf("layer %s digest mismatch: expected %s got %s", layer.Digest, layer.Digest, expectedDigest)
				}
				if err := uploadBlob(layer.Digest, layerData); err != nil {
					return fmt.Errorf("upload layer %s: %w", layer.Digest, err)
				}
			}
		}
	}

	// upload manifest
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	manifestURL := fmt.Sprintf("https://%s/v2/%s/manifests/%s", registry, name, tag)
	mediaType := "application/vnd.oci.image.manifest.v1+json"
	resp, err := c.doAuthRequest("PUT", manifestURL, map[string]string{
		"Content-Type": mediaType,
	}, bytes.NewReader(manifestJSON))
	if err != nil {
		return fmt.Errorf("put manifest: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("put manifest: status %d body=%s", resp.StatusCode, string(body))
	}

	return nil
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
