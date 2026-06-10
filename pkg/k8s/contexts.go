// Code split from client.go; see that file for the Client type and lifecycle.
package k8s

import (
	"fmt"
	"path/filepath"

	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/util/homedir"
)

// ContextDetail contains metadata about a kubeconfig context.
type ContextDetail struct {
	Name      string `json:"name"`
	Cluster   string `json:"cluster"`
	Server    string `json:"server"`
	AuthInfo  string `json:"authInfo"`
	Namespace string `json:"namespace"`
	IsActive  bool   `json:"isActive"`
}

// FullContextDetail contains all editable fields for a kubeconfig context.
type FullContextDetail struct {
	Name      string `json:"name"`
	Cluster   string `json:"cluster"`
	AuthInfo  string `json:"authInfo"`
	Namespace string `json:"namespace"`
	IsActive  bool   `json:"isActive"`

	ClusterDetail ClusterDetail `json:"clusterDetail"`
	AuthDetail    AuthDetail    `json:"authDetail"`
}

// ClusterDetail contains all cluster-level kubeconfig fields.
type ClusterDetail struct {
	Server                string `json:"server"`
	CertificateAuthority  string `json:"certificateAuthority"`
	InsecureSkipTLSVerify bool   `json:"insecureSkipTLSVerify"`
	ProxyURL              string `json:"proxyURL"`
	TLSServerName         string `json:"tlsServerName"`
	DisableCompression    bool   `json:"disableCompression"`
}

// ExecEnvVar is a name/value pair for exec plugin environment variables.
type ExecEnvVar struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// AuthDetail contains all auth-level kubeconfig fields.
type AuthDetail struct {
	ClientCertificate string `json:"clientCertificate"`
	ClientKey         string `json:"clientKey"`

	Token     string `json:"token"`
	TokenFile string `json:"tokenFile"`

	Username string `json:"username"`

	Impersonate       string   `json:"impersonate"`
	ImpersonateGroups []string `json:"impersonateGroups"`

	HasExecProvider    bool         `json:"hasExecProvider"`
	ExecAPIVersion     string       `json:"execAPIVersion"`
	ExecCommand        string       `json:"execCommand"`
	ExecArgs           []string     `json:"execArgs"`
	ExecEnv            []ExecEnvVar `json:"execEnv"`
	ExecInstallHint    string       `json:"execInstallHint"`
	ExecProvideCluster bool         `json:"execProvideCluster"`

	HasAuthProvider  bool   `json:"hasAuthProvider"`
	AuthProviderName string `json:"authProviderName"`
}

// ContextUpdateRequest contains the fields to update on a kubeconfig context.
// Only non-nil pointer fields are applied.
type ContextUpdateRequest struct {
	Namespace             *string `json:"namespace"`
	Server                *string `json:"server"`
	CertificateAuthority  *string `json:"certificateAuthority"`
	InsecureSkipTLSVerify *bool   `json:"insecureSkipTLSVerify"`
	ProxyURL              *string `json:"proxyURL"`
	TLSServerName         *string `json:"tlsServerName"`
	DisableCompression    *bool   `json:"disableCompression"`

	ClientCertificate *string `json:"clientCertificate"`
	ClientKey         *string `json:"clientKey"`
	Token             *string `json:"token"`
	TokenFile         *string `json:"tokenFile"`
	Username          *string `json:"username"`
	Impersonate       *string `json:"impersonate"`

	ExecCommand     *string      `json:"execCommand"`
	ExecArgs        []string     `json:"execArgs"`
	ExecEnv         []ExecEnvVar `json:"execEnv"`
	ExecInstallHint *string      `json:"execInstallHint"`
	SetExec         bool         `json:"setExec"`
}

// GetContextDetails returns detailed info for all kubeconfig contexts.
func (c *Client) GetContextDetails() ([]ContextDetail, error) {
	loader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		c.getLoadingRules(), &clientcmd.ConfigOverrides{},
	)
	rawConfig, err := loader.RawConfig()
	if err != nil {
		return nil, err
	}

	currentCtx := c.GetCurrentContext()
	details := make([]ContextDetail, 0, len(rawConfig.Contexts))
	for name, ctx := range rawConfig.Contexts {
		d := ContextDetail{
			Name:      name,
			Cluster:   ctx.Cluster,
			AuthInfo:  ctx.AuthInfo,
			Namespace: ctx.Namespace,
			IsActive:  name == currentCtx,
		}
		if cluster, ok := rawConfig.Clusters[ctx.Cluster]; ok {
			d.Server = cluster.Server
		}
		details = append(details, d)
	}
	return details, nil
}

// DeleteContext removes a context from the kubeconfig file.
func (c *Client) DeleteContext(name string) error {
	if name == c.GetCurrentContext() {
		return fmt.Errorf("cannot delete the active context %q; switch to another context first", name)
	}

	// Find and modify the kubeconfig file containing this context
	return c.modifyKubeconfigContext(name, func(config *clientcmdapi.Config) error {
		if _, ok := config.Contexts[name]; !ok {
			return fmt.Errorf("context %q not found", name)
		}
		delete(config.Contexts, name)
		if config.CurrentContext == name {
			config.CurrentContext = ""
		}
		return nil
	})
}

// RenameContext renames a context in the kubeconfig file.
func (c *Client) RenameContext(oldName, newName string) error {
	if newName == "" {
		return fmt.Errorf("new context name cannot be empty")
	}
	if oldName == newName {
		return nil
	}

	err := c.modifyKubeconfigContext(oldName, func(config *clientcmdapi.Config) error {
		if _, ok := config.Contexts[oldName]; !ok {
			return fmt.Errorf("context %q not found", oldName)
		}
		if _, exists := config.Contexts[newName]; exists {
			return fmt.Errorf("context %q already exists", newName)
		}
		config.Contexts[newName] = config.Contexts[oldName]
		delete(config.Contexts, oldName)
		if config.CurrentContext == oldName {
			config.CurrentContext = newName
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Update internal state if renaming the active context
	if oldName == c.GetCurrentContext() {
		c.mu.Lock()
		c.currentContext = newName
		c.mu.Unlock()
	}
	return nil
}

// modifyKubeconfigContext finds the kubeconfig file containing the named context,
// applies the mutation, and writes the file back.
func (c *Client) modifyKubeconfigContext(contextName string, mutate func(*clientcmdapi.Config) error) error {
	home := homedir.HomeDir()
	primary := filepath.Join(home, ".kube", "config")

	c.mu.RLock()
	extra := c.extraKubeconfigPaths
	c.mu.RUnlock()

	allPaths := append([]string{primary}, extra...)

	for _, path := range allPaths {
		rules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: path}
		loader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &clientcmd.ConfigOverrides{})
		rawConfig, err := loader.RawConfig()
		if err != nil {
			continue
		}
		if _, ok := rawConfig.Contexts[contextName]; !ok {
			continue
		}
		// Found the file — apply mutation
		if err := mutate(&rawConfig); err != nil {
			return err
		}
		return clientcmd.WriteToFile(rawConfig, path)
	}

	return fmt.Errorf("context %q not found in any kubeconfig file", contextName)
}

// GetFullContextDetail returns all editable fields for a kubeconfig context.
func (c *Client) GetFullContextDetail(name string) (*FullContextDetail, error) {
	loader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		c.getLoadingRules(), &clientcmd.ConfigOverrides{},
	)
	rawConfig, err := loader.RawConfig()
	if err != nil {
		return nil, err
	}

	ctx, ok := rawConfig.Contexts[name]
	if !ok {
		return nil, fmt.Errorf("context %q not found", name)
	}

	detail := &FullContextDetail{
		Name:      name,
		Cluster:   ctx.Cluster,
		AuthInfo:  ctx.AuthInfo,
		Namespace: ctx.Namespace,
		IsActive:  name == c.GetCurrentContext(),
	}

	if cluster, ok := rawConfig.Clusters[ctx.Cluster]; ok {
		detail.ClusterDetail = ClusterDetail{
			Server:                cluster.Server,
			CertificateAuthority:  cluster.CertificateAuthority,
			InsecureSkipTLSVerify: cluster.InsecureSkipTLSVerify,
			ProxyURL:              cluster.ProxyURL,
			TLSServerName:         cluster.TLSServerName,
			DisableCompression:    cluster.DisableCompression,
		}
	}

	if auth, ok := rawConfig.AuthInfos[ctx.AuthInfo]; ok {
		detail.AuthDetail = AuthDetail{
			ClientCertificate: auth.ClientCertificate,
			ClientKey:         auth.ClientKey,
			Token:             auth.Token,
			TokenFile:         auth.TokenFile,
			Username:          auth.Username,
			Impersonate:       auth.Impersonate,
			ImpersonateGroups: auth.ImpersonateGroups,
		}
		if auth.Exec != nil {
			detail.AuthDetail.HasExecProvider = true
			detail.AuthDetail.ExecAPIVersion = auth.Exec.APIVersion
			detail.AuthDetail.ExecCommand = auth.Exec.Command
			detail.AuthDetail.ExecArgs = auth.Exec.Args
			detail.AuthDetail.ExecInstallHint = auth.Exec.InstallHint
			detail.AuthDetail.ExecProvideCluster = auth.Exec.ProvideClusterInfo
			envVars := make([]ExecEnvVar, 0, len(auth.Exec.Env))
			for _, ev := range auth.Exec.Env {
				envVars = append(envVars, ExecEnvVar{Name: ev.Name, Value: ev.Value})
			}
			detail.AuthDetail.ExecEnv = envVars
		}
		if auth.AuthProvider != nil {
			detail.AuthDetail.HasAuthProvider = true
			detail.AuthDetail.AuthProviderName = auth.AuthProvider.Name
		}
	}

	return detail, nil
}

// UpdateContextDetail applies partial updates to a kubeconfig context.
func (c *Client) UpdateContextDetail(name string, req ContextUpdateRequest) error {
	return c.modifyKubeconfigContext(name, func(config *clientcmdapi.Config) error {
		ctx, ok := config.Contexts[name]
		if !ok {
			return fmt.Errorf("context %q not found", name)
		}

		// Context-level fields
		if req.Namespace != nil {
			ctx.Namespace = *req.Namespace
		}

		// Cluster-level fields
		if cluster, ok := config.Clusters[ctx.Cluster]; ok {
			if req.Server != nil {
				cluster.Server = *req.Server
			}
			if req.CertificateAuthority != nil {
				cluster.CertificateAuthority = *req.CertificateAuthority
			}
			if req.InsecureSkipTLSVerify != nil {
				cluster.InsecureSkipTLSVerify = *req.InsecureSkipTLSVerify
			}
			if req.ProxyURL != nil {
				cluster.ProxyURL = *req.ProxyURL
			}
			if req.TLSServerName != nil {
				cluster.TLSServerName = *req.TLSServerName
			}
			if req.DisableCompression != nil {
				cluster.DisableCompression = *req.DisableCompression
			}
		}

		// Auth-level fields
		if auth, ok := config.AuthInfos[ctx.AuthInfo]; ok {
			if req.ClientCertificate != nil {
				auth.ClientCertificate = *req.ClientCertificate
			}
			if req.ClientKey != nil {
				auth.ClientKey = *req.ClientKey
			}
			if req.Token != nil {
				auth.Token = *req.Token
			}
			if req.TokenFile != nil {
				auth.TokenFile = *req.TokenFile
			}
			if req.Username != nil {
				auth.Username = *req.Username
			}
			if req.Impersonate != nil {
				auth.Impersonate = *req.Impersonate
			}

			// Exec plugin fields
			if req.SetExec {
				if auth.Exec == nil {
					auth.Exec = &clientcmdapi.ExecConfig{}
				}
				if req.ExecCommand != nil {
					auth.Exec.Command = *req.ExecCommand
				}
				if req.ExecArgs != nil {
					auth.Exec.Args = req.ExecArgs
				}
				if req.ExecEnv != nil {
					envVars := make([]clientcmdapi.ExecEnvVar, 0, len(req.ExecEnv))
					for _, ev := range req.ExecEnv {
						envVars = append(envVars, clientcmdapi.ExecEnvVar{Name: ev.Name, Value: ev.Value})
					}
					auth.Exec.Env = envVars
				}
				if req.ExecInstallHint != nil {
					auth.Exec.InstallHint = *req.ExecInstallHint
				}
			}
		}

		return nil
	})
}
