package module

import (
	"os"
	"strings"
)

// K8s reads a kubeconfig file and returns "<current-context>/<namespace>", or
// just "<current-context>" if no namespace is set, or "" on any problem. This
// is a deliberately small line-scan (no kubectl, no YAML lib) sufficient for the
// common single-document kubeconfig.
func K8s(kubeconfigPath string) string {
	data, err := os.ReadFile(kubeconfigPath)
	if err != nil {
		return ""
	}
	var current, namespace string
	for _, line := range strings.Split(string(data), "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "current-context:") {
			current = strings.TrimSpace(strings.TrimPrefix(t, "current-context:"))
		}
		if strings.HasPrefix(t, "namespace:") {
			namespace = strings.TrimSpace(strings.TrimPrefix(t, "namespace:"))
		}
	}
	if current == "" {
		return ""
	}
	if namespace == "" {
		return current
	}
	return current + "/" + namespace
}
