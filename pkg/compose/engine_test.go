package compose

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEngineNew(t *testing.T) {
	e := NewEngine("testproject", nil, nil, nil)
	if e == nil {
		t.Fatal("NewEngine returned nil")
	}
	if e.project != "testproject" {
		t.Errorf("project = %q, want testproject", e.project)
	}
}

func TestEngineLoadNonexistent(t *testing.T) {
	e := NewEngine("test", nil, nil, nil)
	err := e.Load("/tmp/doki-compose-test-nonexistent-dir")
	if err == nil {
		t.Fatal("expected error for nonexistent dir")
	}
}

func TestEngineLoadValidYAML(t *testing.T) {
	dir := t.TempDir()
	yamlContent := []byte(`
version: "3.8"
name: myapp
services:
  web:
    image: nginx:alpine
    ports:
      - "8080:80"
    depends_on:
      - api
  api:
    image: python:3-alpine
    command: python app.py
    environment:
      DATABASE_URL: postgresql://localhost/db
`)
	yamlPath := filepath.Join(dir, "doki.yml")
	if err := os.WriteFile(yamlPath, yamlContent, 0644); err != nil {
		t.Fatalf("write yaml: %v", err)
	}

	e := NewEngine("test", nil, nil, nil)
	if err := e.Load(dir); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if e.file == nil {
		t.Fatal("file is nil after Load")
	}
	if e.file.Version != "3.8" {
		t.Errorf("Version = %q, want 3.8", e.file.Version)
	}
	if e.file.Name != "myapp" {
		t.Errorf("Name = %q, want myapp", e.file.Name)
	}
	if len(e.file.Services) != 2 {
		t.Errorf("len(Services) = %d, want 2", len(e.file.Services))
	}
	if e.file.Services["web"] == nil {
		t.Error("web service is nil")
	}
	if e.file.Services["web"].Image != "nginx:alpine" {
		t.Errorf("web.Image = %q, want nginx:alpine", e.file.Services["web"].Image)
	}
	if len(e.file.Services["web"].Ports) != 1 {
		t.Errorf("len(web.Ports) = %d, want 1", len(e.file.Services["web"].Ports))
	}
}

func TestEngineLoadDockerComposeYML(t *testing.T) {
	dir := t.TempDir()
	yamlContent := []byte(`
services:
  app:
    image: myapp:latest
    build:
      context: .
      dockerfile: Dockerfile
`)
	yamlPath := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(yamlPath, yamlContent, 0644); err != nil {
		t.Fatalf("write yaml: %v", err)
	}

	e := NewEngine("test", nil, nil, nil)
	if err := e.Load(dir); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if e.file.Services["app"].Build == nil {
		t.Error("build config is nil")
	}
	if e.file.Services["app"].Build.Context != "." {
		t.Errorf("Build.Context = %q, want .", e.file.Services["app"].Build.Context)
	}
}

func TestEngineLoadWithNetworks(t *testing.T) {
	dir := t.TempDir()
	yamlContent := []byte(`
services:
  app:
    image: alpine
    networks:
      - frontend
      - backend
networks:
  frontend:
    driver: bridge
  backend:
    driver: bridge
    internal: true
`)
	yamlPath := filepath.Join(dir, "compose.yml")
	if err := os.WriteFile(yamlPath, yamlContent, 0644); err != nil {
		t.Fatalf("write yaml: %v", err)
	}

	e := NewEngine("test", nil, nil, nil)
	if err := e.Load(dir); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(e.file.Networks) != 2 {
		t.Errorf("len(Networks) = %d, want 2", len(e.file.Networks))
	}
}

func TestEngineLoadWithVolumes(t *testing.T) {
	dir := t.TempDir()
	yamlContent := []byte(`
services:
  db:
    image: mysql:8
    volumes:
      - db-data:/var/lib/mysql
volumes:
  db-data:
    driver: local
`)
	yamlPath := filepath.Join(dir, "doki-compose.yml")
	if err := os.WriteFile(yamlPath, yamlContent, 0644); err != nil {
		t.Fatalf("write yaml: %v", err)
	}

	e := NewEngine("test", nil, nil, nil)
	if err := e.Load(dir); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(e.file.Volumes) != 1 {
		t.Errorf("len(Volumes) = %d, want 1", len(e.file.Volumes))
	}
	if e.file.Volumes["db-data"].Driver != "local" {
		t.Errorf("Volume driver = %q, want local", e.file.Volumes["db-data"].Driver)
	}
}

func TestOrderServicesNoDeps(t *testing.T) {
	e := &Engine{
		file: &ComposeFile{
			Services: map[string]*Service{
				"web": {},
				"api": {},
				"db":  {},
			},
		},
	}
	ordered := e.orderServices()
	if len(ordered) != 3 {
		t.Fatalf("len(ordered) = %d, want 3", len(ordered))
	}
	for _, svc := range []string{"web", "api", "db"} {
		found := false
		for _, o := range ordered {
			if o == svc {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("service %q not in ordered list: %v", svc, ordered)
		}
	}
}

func TestOrderServicesWithDeps(t *testing.T) {
	e := &Engine{
		file: &ComposeFile{
			Services: map[string]*Service{
				"web": {DependsOn: []interface{}{"api"}},
				"api": {DependsOn: []interface{}{"db"}},
				"db":  {},
			},
		},
	}
	ordered := e.orderServices()

	// db should come before api, api before web.
	dbIdx := indexOf(ordered, "db")
	apiIdx := indexOf(ordered, "api")
	webIdx := indexOf(ordered, "web")

	if dbIdx > apiIdx {
		t.Errorf("db (idx %d) should come before api (idx %d)", dbIdx, apiIdx)
	}
	if apiIdx > webIdx {
		t.Errorf("api (idx %d) should come before web (idx %d)", apiIdx, webIdx)
	}
}

func indexOf(slice []string, item string) int {
	for i, s := range slice {
		if s == item {
			return i
		}
	}
	return -1
}

func TestOrderServicesWithMapDeps(t *testing.T) {
	e := &Engine{
		file: &ComposeFile{
			Services: map[string]*Service{
				"web": {DependsOn: map[string]interface{}{
					"api": map[string]interface{}{"condition": "service_started"},
				}},
				"api": {},
			},
		},
	}
	ordered := e.orderServices()
	apiIdx := indexOf(ordered, "api")
	webIdx := indexOf(ordered, "web")
	if apiIdx > webIdx {
		t.Errorf("api (idx %d) should come before web (idx %d)", apiIdx, webIdx)
	}
}

func TestToStringSlice(t *testing.T) {
	if s := toStringSlice(nil); s != nil {
		t.Errorf("nil -> %v, want nil", s)
	}
	if s := toStringSlice("hello"); len(s) != 1 || s[0] != "hello" {
		t.Errorf("string -> %v, want [hello]", s)
	}
	if s := toStringSlice([]string{"a", "b"}); len(s) != 2 || s[0] != "a" {
		t.Errorf("[]string -> %v, want [a b]", s)
	}
	if s := toStringSlice([]interface{}{"x", "y"}); len(s) != 2 || s[0] != "x" {
		t.Errorf("[]interface{} -> %v, want [x y]", s)
	}
}

func TestToEnvSlice(t *testing.T) {
	if s := toEnvSlice(nil); s != nil {
		t.Errorf("nil -> %v, want nil", s)
	}
	if s := toEnvSlice([]string{"KEY=val"}); len(s) != 1 || s[0] != "KEY=val" {
		t.Errorf("[]string -> %v", s)
	}
	if s := toEnvSlice(map[string]interface{}{
		"HOST": "localhost", "PORT": 8080,
	}); len(s) != 2 {
		t.Errorf("map -> %d elements, want 2", len(s))
	}
}

func TestEngineUpNoFile(t *testing.T) {
	e := NewEngine("test", nil, nil, nil)
	err := e.Up()
	if err == nil {
		t.Fatal("expected error for Up without loaded file")
	}
}

func TestEngineDownNoFile(t *testing.T) {
	e := NewEngine("test", nil, nil, nil)
	err := e.Down()
	if err == nil {
		t.Fatal("expected error for Down without loaded file")
	}
}
