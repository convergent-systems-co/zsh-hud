package module

import (
	"os"
	"path/filepath"
	"testing"
)

func TestK8sReadsCurrentContext(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	cfg := "apiVersion: v1\ncurrent-context: aks-dev\ncontexts:\n- name: aks-dev\n  context:\n    namespace: default\n"
	if err := os.WriteFile(path, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := K8s(path); got != "aks-dev/default" {
		t.Fatalf("K8s = %q, want 'aks-dev/default'", got)
	}
}

func TestK8sEmptyWhenFileMissing(t *testing.T) {
	if got := K8s(filepath.Join(t.TempDir(), "nope")); got != "" {
		t.Fatalf("K8s missing file = %q, want empty", got)
	}
}
