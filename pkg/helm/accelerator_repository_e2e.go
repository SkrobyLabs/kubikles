//go:build helm && accelerator_e2e

package helm

import (
	"net"
	"os"
	"strconv"
	"strings"
)

const acceleratorE2EImageRepositoryEnvironment = "ACCELERATOR_PROVISION_KIND_IMAGE_REPOSITORY"

func acceleratorRuntimeImageRepository() string {
	repository, configured := os.LookupEnv(acceleratorE2EImageRepositoryEnvironment)
	if !configured {
		return acceleratorImageRepository
	}
	registry, path, found := strings.Cut(repository, "/")
	if !found || path != "skrobylabs/kubikles-accelerator" {
		return ""
	}
	host, port, err := net.SplitHostPort(registry)
	if err != nil || (host != "127.0.0.1" && host != "host.docker.internal") {
		return ""
	}
	numericPort, err := strconv.Atoi(port)
	if err != nil || numericPort < 1 || numericPort > 65535 || port != strconv.Itoa(numericPort) {
		return ""
	}
	return repository
}
