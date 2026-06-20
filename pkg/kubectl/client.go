// Package kubectl provides a kubectl-compatible HTTP client.
package kubectl

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Client struct {
	server    string
	namespace string
	token     string
	client    *http.Client
}

func NewClient(server string) *Client {
	if server == "" {
		server = os.Getenv("DOKI_KUBE_SERVER")
		if server == "" {
			server = "http://localhost:6443"
		}
	}
	return &Client{
		server:    server,
		namespace: "default",
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) SetNamespace(ns string) {
	c.namespace = ns
}

func (c *Client) SetToken(token string) {
	c.token = token
}

func (c *Client) doRequest(method, path string, body io.Reader) (*http.Response, error) {
	url := c.server + path
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	return c.client.Do(req)
}

func (c *Client) Get(resource, name string) (map[string]interface{}, error) {
	path := c.resourcePath(resource, c.namespace)
	if name != "" {
		path += "/" + name
	}
	resp, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s returned %d", path, resp.StatusCode)
	}
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) List(resource string) ([]map[string]interface{}, error) {
	path := c.resourcePath(resource, c.namespace)
	resp, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s returned %d", path, resp.StatusCode)
	}
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	items, ok := result["items"].([]interface{})
	if !ok {
		return nil, nil
	}
	var list []map[string]interface{}
	for _, item := range items {
		if m, ok := item.(map[string]interface{}); ok {
			list = append(list, m)
		}
	}
	return list, nil
}

func (c *Client) Apply(filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	var obj map[string]interface{}
	if err := yaml.Unmarshal(data, &obj); err != nil {
		return err
	}
	jsonData, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	kind := strings.ToLower(fmt.Sprintf("%v", obj["kind"]))
	meta := obj["metadata"].(map[string]interface{})
	name := fmt.Sprintf("%v", meta["name"])
	namespace := c.namespace
	if ns, ok := meta["namespace"]; ok {
		namespace = fmt.Sprintf("%v", ns)
	}
	path := c.resourcePath(kind, namespace) + "/" + name
	resp, err := c.doRequest("PUT", path, bytes.NewReader(jsonData))
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("PUT %s returned %d", path, resp.StatusCode)
	}
	return nil
}

func (c *Client) Delete(resource, name string) error {
	path := c.resourcePath(resource, c.namespace) + "/" + name
	resp, err := c.doRequest("DELETE", path, nil)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("DELETE %s returned %d", path, resp.StatusCode)
	}
	return nil
}

func (c *Client) Logs(pod string) (string, error) {
	path := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/log", c.namespace, pod)
	resp, err := c.doRequest("GET", path, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (c *Client) Watch(resource string) (<-chan map[string]interface{}, error) {
	path := c.resourcePath(resource, c.namespace) + "?watch=true"
	u, err := url.Parse(c.server + path)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	ch := make(chan map[string]interface{}, 100)
	go func() {
		defer func() { _ = resp.Body.Close() }()
		defer close(ch)
		decoder := json.NewDecoder(resp.Body)
		for {
			var event map[string]interface{}
			if err := decoder.Decode(&event); err != nil {
				return
			}
			ch <- event
		}
	}()
	return ch, nil
}

func (c *Client) resourcePath(resource, namespace string) string {
	switch resource {
	case "pods", "services", "configmaps", "secrets", "serviceaccounts", "endpoints", "events", "persistentvolumeclaims":
		return fmt.Sprintf("/api/v1/namespaces/%s/%s", namespace, resource)
	case "namespaces", "nodes", "persistentvolumes":
		return fmt.Sprintf("/api/v1/%s", resource)
	case "deployments", "replicasets", "statefulsets", "daemonsets":
		return fmt.Sprintf("/apis/apps/v1/namespaces/%s/%s", namespace, resource)
	case "jobs", "cronjobs":
		return fmt.Sprintf("/apis/batch/v1/namespaces/%s/%s", namespace, resource)
	case "ingresses", "networkpolicies":
		return fmt.Sprintf("/apis/networking/v1/namespaces/%s/%s", namespace, resource)
	case "roles", "rolebindings":
		return fmt.Sprintf("/apis/rbac/v1/namespaces/%s/%s", namespace, resource)
	case "clusterroles", "clusterrolebindings":
		return fmt.Sprintf("/apis/rbac/v1/%s", resource)
	default:
		return fmt.Sprintf("/api/v1/namespaces/%s/%s", namespace, resource)
	}
}

func (c *Client) Version() (map[string]interface{}, error) {
	resp, err := c.doRequest("GET", "/version", nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) APIResources() ([]map[string]interface{}, error) {
	resp, err := c.doRequest("GET", "/api/v1", nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	resources, ok := result["resources"].([]interface{})
	if !ok {
		return nil, nil
	}
	var list []map[string]interface{}
	for _, r := range resources {
		if m, ok := r.(map[string]interface{}); ok {
			list = append(list, m)
		}
	}
	return list, nil
}
