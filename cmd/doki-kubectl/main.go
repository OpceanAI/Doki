package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/OpceanAI/Doki/pkg/common"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	server := getServer()
	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "get":
		cmdGet(server, args)
	case "apply":
		cmdApply(server, args)
	case "delete":
		cmdDelete(server, args)
	case "describe":
		cmdDescribe(server, args)
	case "logs":
		cmdLogs(server, args)
	case "version":
		cmdVersion(server)
	case "cluster-info":
		cmdClusterInfo(server)
	case "api-resources":
		cmdAPIResources(server)
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

func getServer() string {
	if s := os.Getenv("DOKI_KUBE_SERVER"); s != "" {
		return s
	}
	if s := os.Getenv("KUBE_API_SERVER"); s != "" {
		return s
	}
	return "http://localhost:6443"
}

func printUsage() {
	fmt.Println(`Usage: doki-kubectl <command> [args]

Commands:
  get <resource> [name]     Get resources
  apply -f <file>           Apply configuration
  delete <resource> <name>  Delete resource
  describe <resource> <name> Show details
  logs <pod>                Show pod logs
  version                   Show version
  cluster-info              Show cluster info
  api-resources             List API resources
  help                      Show this help`)
}

func cmdGet(server string, args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: doki-kubectl get <resource> [name]")
		os.Exit(1)
	}

	resource := args[0]
	namespace := getNamespace()

	var url string
	if len(args) >= 2 {
		url = fmt.Sprintf("%s/api/v1/namespaces/%s/%s/%s", server, namespace, resource, args[1])
	} else {
		if isClusterScoped(resource) {
			url = fmt.Sprintf("%s/api/v1/%s", server, resource)
		} else {
			url = fmt.Sprintf("%s/api/v1/namespaces/%s/%s", server, namespace, resource)
		}
	}

	resp, err := http.Get(url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)

	if len(args) >= 2 {
		var obj map[string]interface{}
		_ = json.Unmarshal(body, &obj)
		printResource(obj)
	} else {
		var list map[string]interface{}
		_ = json.Unmarshal(body, &list)
		printResourceList(list, resource)
	}
}

func cmdApply(server string, args []string) {
	file := ""
	for i, arg := range args {
		if (arg == "-f" || arg == "--filename") && i+1 < len(args) {
			file = args[i+1]
		}
	}
	if file == "" {
		fmt.Fprintln(os.Stderr, "usage: doki-kubectl apply -f <file>")
		os.Exit(1)
	}

	data, err := os.ReadFile(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading file: %v\n", err)
		os.Exit(1)
	}

	var obj map[string]interface{}
	if err := json.Unmarshal(data, &obj); err != nil {
		fmt.Fprintf(os.Stderr, "error parsing JSON: %v\n", err)
		os.Exit(1)
	}

	kind := strings.ToLower(fmt.Sprintf("%v", obj["kind"]))
	meta := obj["metadata"].(map[string]interface{})
	name := fmt.Sprintf("%v", meta["name"])
	namespace := "default"
	if ns, ok := meta["namespace"]; ok {
		namespace = fmt.Sprintf("%v", ns)
	}

	url := getResourceURL(server, kind, namespace, name)
	req, err := http.NewRequest(http.MethodPut, url, strings.NewReader(string(data)))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating request: %v\n", err)
		os.Exit(1)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusCreated {
		fmt.Printf("%s/%s created\n", kind, name)
	} else {
		fmt.Printf("%s/%s configured\n", kind, name)
	}
}

func cmdDelete(server string, args []string) {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: doki-kubectl delete <resource> <name>")
		os.Exit(1)
	}

	resource := args[0]
	name := args[1]
	namespace := getNamespace()

	url := getResourceURL(server, resource, namespace, name)
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating request: %v\n", err)
		os.Exit(1)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = resp.Body.Close() }()

	fmt.Printf("%s/%s deleted\n", resource, name)
}

func cmdDescribe(server string, args []string) {
	cmdGet(server, args)
}

func cmdLogs(server string, args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: doki-kubectl logs <pod>")
		os.Exit(1)
	}
	pod := args[0]
	namespace := getNamespace()
	url := fmt.Sprintf("%s/api/v1/namespaces/%s/pods/%s/log", server, namespace, pod)
	resp, err := http.Get(url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(os.Stdout, resp.Body)
}

func cmdVersion(server string) {
	resp, err := http.Get(server + "/version")
	if err != nil {
		conn, err := net.Dial("unix", "/var/run/doki-cri.sock")
		if err != nil {
			fmt.Printf("Client Version: v%s\n", common.DokiVersion)
			fmt.Println("Server: not reachable")
			return
		}
		_ = conn.Close()
		fmt.Printf("Client Version: v%s\n", common.DokiVersion)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("Client Version: v0.10.0\n")
	fmt.Printf("Server Version: %s\n", string(body))
}

func cmdClusterInfo(server string) {
	fmt.Printf("Kubernetes control plane is running at %s\n", server)
	fmt.Printf("CoreDNS is running at %s/api/v1/namespaces/kube-system/services/kube-dns/proxy\n", server)
}

func cmdAPIResources(server string) {
	resp, err := http.Get(server + "/api/v1")
	if err != nil {
		fmt.Fprintln(os.Stderr, "error: cannot reach API server")
		os.Exit(1)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)

	var result map[string]interface{}
	_ = json.Unmarshal(body, &result)

	fmt.Printf("%-30s %-15s %-15s\n", "NAME", "KIND", "NAMESPACED")
	if resources, ok := result["resources"].([]interface{}); ok {
		for _, r := range resources {
			res := r.(map[string]interface{})
			namespaced := fmt.Sprintf("%v", res["namespaced"])
			fmt.Printf("%-30s %-15s %-15s\n", res["name"], res["kind"], namespaced)
		}
	}
}

func getNamespace() string {
	if ns := os.Getenv("DOKI_KUBE_NAMESPACE"); ns != "" {
		return ns
	}
	return "default"
}

func isClusterScoped(resource string) bool {
	switch resource {
	case "namespaces", "nodes", "persistentvolumes":
		return true
	}
	return false
}

func getResourceURL(server, resource, namespace, name string) string {
	switch resource {
	case "deployments", "replicasets", "statefulsets", "daemonsets":
		return fmt.Sprintf("%s/apis/apps/v1/namespaces/%s/%s/%s", server, namespace, resource, name)
	case "jobs", "cronjobs":
		return fmt.Sprintf("%s/apis/batch/v1/namespaces/%s/%s/%s", server, namespace, resource, name)
	case "ingresses", "networkpolicies":
		return fmt.Sprintf("%s/apis/networking/v1/namespaces/%s/%s/%s", server, namespace, resource, name)
	case "roles", "rolebindings":
		return fmt.Sprintf("%s/apis/rbac/v1/namespaces/%s/%s/%s", server, namespace, resource, name)
	case "clusterroles", "clusterrolebindings":
		return fmt.Sprintf("%s/apis/rbac/v1/%s/%s", server, resource, name)
	case "namespaces", "nodes", "persistentvolumes":
		return fmt.Sprintf("%s/api/v1/%s/%s", server, resource, name)
	default:
		return fmt.Sprintf("%s/api/v1/namespaces/%s/%s/%s", server, namespace, resource, name)
	}
}

func printResource(obj map[string]interface{}) {
	meta := obj["metadata"].(map[string]interface{})
	fmt.Printf("Name:      %v\n", meta["name"])
	if ns, ok := meta["namespace"]; ok {
		fmt.Printf("Namespace: %v\n", ns)
	}
	fmt.Printf("Kind:      %v\n", obj["kind"])
	if status, ok := obj["status"]; ok {
		fmt.Printf("Status:    %v\n", status)
	}
}

func printResourceList(list map[string]interface{}, resource string) {
	items, ok := list["items"].([]interface{})
	if !ok {
		fmt.Println("No resources found.")
		return
	}

	fmt.Printf("%-40s %-15s %-20s\n", "NAME", "STATUS", "AGE")
	for _, item := range items {
		obj := item.(map[string]interface{})
		meta := obj["metadata"].(map[string]interface{})
		name := fmt.Sprintf("%v", meta["name"])
		status := "Active"
		if s, ok := obj["status"]; ok {
			if sm, ok := s.(map[string]interface{}); ok {
				if phase, ok := sm["phase"]; ok {
					status = fmt.Sprintf("%v", phase)
				}
			}
		}
		fmt.Printf("%-40s %-15s %-20s\n", name, status, "<unknown>")
	}
}
