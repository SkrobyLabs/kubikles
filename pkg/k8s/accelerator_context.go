package k8s

// This file deliberately uses a fresh deferred loader.  A provisioning attempt
// must not borrow or mutate the client's selected connection.

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"

	"kubikles/pkg/debug"

	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var ErrAcceleratorContextUnavailable = errors.New("accelerator context unavailable")

// AcceleratorContextSnapshot is an immutable, credential-free description of
// the selected desktop context. Identity is intentionally private to callers.
type AcceleratorContextSnapshot struct {
	contextName string
	namespace   string
	identity    string
	restConfig  *rest.Config
	clientset   kubernetes.Interface
}

func (s AcceleratorContextSnapshot) String() string { return "<accelerator context snapshot>" }
func (s AcceleratorContextSnapshot) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "<accelerator context snapshot>")
}

func (s *AcceleratorContextSnapshot) ContextName() string {
	if s == nil {
		return ""
	}
	return s.contextName
}
func (s *AcceleratorContextSnapshot) Namespace() string {
	if s == nil {
		return ""
	}
	return s.namespace
}
func (s *AcceleratorContextSnapshot) Identity() string {
	if s == nil {
		return ""
	}
	return s.identity
}
func (s *AcceleratorContextSnapshot) RESTConfig() *rest.Config {
	if s == nil {
		return nil
	}
	return deepCopyRESTConfig(s.restConfig)
}
func (s *AcceleratorContextSnapshot) Clientset() kubernetes.Interface {
	if s == nil {
		return nil
	}
	return s.clientset
}

func acceleratorIdentity(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		fmt.Fprintf(h, "%d:", len(p))
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// SnapshotCurrentContext resolves exactly the currently selected merged
// kubeconfig context and creates one client from that immutable configuration.
func (c *Client) SnapshotCurrentContext(contextName string) (*AcceleratorContextSnapshot, error) {
	fail := func(stage, reason string, err error) (*AcceleratorContextSnapshot, error) {
		if err != nil {
			reason = err.Error()
		}
		debug.LogHelm("Accelerator context snapshot failed", map[string]interface{}{
			"context": contextName,
			"stage":   stage,
			"error":   reason,
		})
		return nil, ErrAcceleratorContextUnavailable
	}
	if strings.TrimSpace(contextName) == "" || strings.TrimSpace(contextName) != contextName || IsDebugClusterContext(contextName) || contextName != c.GetCurrentContext() {
		return fail("validate_context", "requested context is empty, malformed, reserved, or not current", nil)
	}
	if c.isFixedContext() {
		return fail("validate_context", "fixed-context mode does not support Accelerator snapshots", nil)
	}
	loader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(c.getLoadingRules(), &clientcmd.ConfigOverrides{CurrentContext: contextName})
	raw, err := loader.RawConfig()
	if err != nil {
		return fail("load_kubeconfig", "", err)
	}
	ctxCfg, ok := raw.Contexts[contextName]
	if !ok || ctxCfg == nil {
		return fail("validate_context_entry", "requested context is missing from kubeconfig", nil)
	}
	cluster, ok := raw.Clusters[ctxCfg.Cluster]
	if !ok || cluster == nil || cluster.Server == "" {
		return fail("validate_cluster", "context cluster is missing or has no server", nil)
	}
	if _, ok := raw.AuthInfos[ctxCfg.AuthInfo]; !ok || ctxCfg.AuthInfo == "" {
		return fail("validate_auth", "context auth info is missing", nil)
	}
	cfg, err := loader.ClientConfig()
	if err != nil {
		return fail("build_client_config", "", err)
	}
	copy := deepCopyRESTConfig(cfg)
	// Resolve file-backed TLS material once while the snapshot is made.  Later
	// file changes must not change the connection or identity under a gate.
	if err := rest.LoadTLSFiles(copy); err != nil {
		return fail("load_tls_files", "", err)
	}
	copy.TLSClientConfig.CAFile = ""
	copy.TLSClientConfig.CertFile = ""
	copy.TLSClientConfig.KeyFile = ""
	cs, err := kubernetes.NewForConfig(copy)
	if err != nil {
		return fail("create_kubernetes_client", "", err)
	}
	ns := ctxCfg.Namespace
	if ns == "" {
		ns = "default"
	}
	if len(validation.IsDNS1123Label(ns)) != 0 {
		return fail("validate_namespace", "context namespace is not a valid DNS-1123 label", nil)
	}
	ca := sha256.Sum256(copy.TLSClientConfig.CAData)
	return &AcceleratorContextSnapshot{contextName: contextName, namespace: ns, restConfig: copy, clientset: cs,
		identity: acceleratorIdentity(contextName, ctxCfg.Cluster, ctxCfg.AuthInfo, cluster.Server, cluster.TLSServerName, hex.EncodeToString(ca[:]))}, nil
}
