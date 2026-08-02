package acceleratorrelease

import "net/url"

const releaseAssetPrefix = "kubikles-accelerator-release-"

func assetURLs(buildVersion string) (descriptorURL, checksumURL string, ok bool) {
	if !stableVersion.MatchString(buildVersion) {
		return "", "", false
	}
	name := releaseAssetPrefix + buildVersion + ".json"
	u := url.URL{
		Scheme: "https",
		Host:   "github.com",
		Path:   "/SkrobyLabs/kubikles/releases/download/" + buildVersion + "/" + name,
	}
	descriptorURL = u.String()
	return descriptorURL, descriptorURL + ".sha256", true
}
