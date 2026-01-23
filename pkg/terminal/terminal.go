package terminal

// BuildKubectlArgs constructs the kubectl exec command arguments
func BuildKubectlArgs(namespace, pod, container, contextName, customCommand string) []string {
	cmdArgs := []string{"exec", "-it"}
	if contextName != "" {
		cmdArgs = append(cmdArgs, "--context", contextName)
	}
	cmdArgs = append(cmdArgs, "-n", namespace, pod)
	if container != "" {
		cmdArgs = append(cmdArgs, "-c", container)
	}
	if customCommand == "nsenter" {
		// Special case for node shell - pass nsenter args directly
		cmdArgs = append(cmdArgs, "--", "nsenter", "-t", "1", "-m", "-u", "-i", "-n", "-p", "--", "/bin/sh")
	} else if customCommand != "" {
		// Use custom command wrapped in shell
		cmdArgs = append(cmdArgs, "--", "/bin/sh", "-c", customCommand)
	} else {
		// Default: try bash, fallback to sh
		cmdArgs = append(cmdArgs, "--", "/bin/sh", "-c", "if command -v bash >/dev/null 2>&1; then exec bash; else exec sh; fi")
	}
	return cmdArgs
}
