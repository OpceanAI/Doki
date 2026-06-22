package compose

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

type PublishConfig struct {
	Registry string
	Tag      string
	Username string
	Password string
}

type OCIManifest struct {
	SchemaVersion int               `json:"schemaVersion"`
	MediaType     string            `json:"mediaType"`
	Config        OCIDescriptor     `json:"config"`
	Layers        []OCIDescriptor   `json:"layers"`
	Annotations   map[string]string `json:"annotations,omitempty"`
}

type OCIDescriptor struct {
	MediaType string `json:"mediaType"`
	Digest    string `json:"digest"`
	Size      int64  `json:"size"`
}

func (e *Engine) Publish(cfg *PublishConfig) error {
	if cfg.Registry == "" {
		return fmt.Errorf("registry is required")
	}

	if e.file == nil {
		return fmt.Errorf("no compose file loaded")
	}

	composeData, err := e.marshalCompose()
	if err != nil {
		return fmt.Errorf("marshal compose: %w", err)
	}

	manifest := &OCIManifest{
		SchemaVersion: 2,
		MediaType:     "application/vnd.oci.image.manifest.v1+json",
		Config: OCIDescriptor{
			MediaType: "application/vnd.docker.compose.v1+yaml",
			Digest:    computeDigest(composeData),
			Size:      int64(len(composeData)),
		},
		Layers: []OCIDescriptor{},
		Annotations: map[string]string{
			"org.opencontainers.image.created": time.Now().Format(time.RFC3339),
			"org.opencontainers.image.title":   e.project,
		},
	}

	manifest.Layers = append(manifest.Layers, OCIDescriptor{
		MediaType: "application/vnd.docker.compose.file+yaml",
		Digest:    computeDigest(composeData),
		Size:      int64(len(composeData)),
	})

	_, err = createArtifactTar(manifest, composeData)
	if err != nil {
		return fmt.Errorf("create artifact: %w", err)
	}

	return nil
}

func (e *Engine) marshalCompose() ([]byte, error) {
	if e.file == nil {
		return nil, fmt.Errorf("no compose file loaded")
	}

	data := map[string]interface{}{
		"name":     e.project,
		"services": e.file.Services,
	}

	if len(e.file.Networks) > 0 {
		data["networks"] = e.file.Networks
	}
	if len(e.file.Volumes) > 0 {
		data["volumes"] = e.file.Volumes
	}
	if len(e.file.Secrets) > 0 {
		data["secrets"] = e.file.Secrets
	}
	if len(e.file.Configs) > 0 {
		data["configs"] = e.file.Configs
	}

	return yaml.Marshal(data)
}

func createArtifactTar(manifest *OCIManifest, composeData []byte) ([]byte, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	manifestJSON, _ := json.MarshalIndent(manifest, "", "  ")
	hdr := &tar.Header{
		Name:    "manifest.json",
		Size:    int64(len(manifestJSON)),
		Mode:    0644,
		ModTime: time.Now(),
	}
	_ = tw.WriteHeader(hdr)
	_, _ = tw.Write(manifestJSON)

	hdr = &tar.Header{
		Name:    "compose.yaml",
		Size:    int64(len(composeData)),
		Mode:    0644,
		ModTime: time.Now(),
	}
	_ = tw.WriteHeader(hdr)
	_, _ = tw.Write(composeData)

	_ = tw.Close()
	return buf.Bytes(), nil
}

func computeDigest(data []byte) string {
	h := sha256.Sum256(data)
	return fmt.Sprintf("sha256:%x", h)
}
