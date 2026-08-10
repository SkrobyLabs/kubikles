package acceleratorsecret

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDecodeHelmReleasePayloadCompressedAndLegacy(t *testing.T) {
	for _, compressed := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy", true: "compressed"}[compressed], func(t *testing.T) {
			got, err := decodeHelmReleasePayload(helmReleasePayloadFixture(t, "example", "team-a", 4, "deployed", time.Date(2026, 8, 10, 19, 30, 0, 0, time.UTC), compressed))
			if err != nil {
				t.Fatal(err)
			}
			if got.Name != "example" || got.Namespace != "team-a" || got.Revision != 4 || got.Status != "deployed" || got.Chart != "example-chart" || got.ChartVersion != "1.2.3" || got.AppVersion != "4.5.6" || got.Description != "Ready" {
				t.Fatalf("release=%#v", got)
			}
			serialized, marshalErr := json.Marshal(got)
			if marshalErr != nil || bytes.Contains(serialized, []byte("must not escape")) {
				t.Fatalf("projection leaked stored fields: %s (%v)", serialized, marshalErr)
			}
		})
	}
}

func TestDecodeHelmReleasePayloadRejectsMalformedAndOversized(t *testing.T) {
	for name, payload := range map[string][]byte{
		"empty": nil, "base64": []byte("not base64"),
		"json":         []byte(base64.StdEncoding.EncodeToString([]byte("not json"))),
		"incomplete":   []byte(base64.StdEncoding.EncodeToString([]byte(`{"name":"example"}`))),
		"payload size": make([]byte, maxHelmReleasePayload+1),
	} {
		t.Run(name, func(t *testing.T) {
			if got, err := decodeHelmReleasePayload(payload); got != nil || !errors.Is(err, errInvalidHelmReleasePayload) {
				t.Fatalf("release=%#v error=%v", got, err)
			}
		})
	}
}

func TestProjectLatestHelmReleaseSecrets(t *testing.T) {
	secret := func(name, namespace string, revision int, status string, updated time.Time) v1.Secret {
		return v1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "sh.helm.release.v1." + name + ".v" + strconv.Itoa(revision), Namespace: namespace}, Type: helmReleaseSecretType, Data: map[string][]byte{"release": helmReleasePayloadFixture(t, name, namespace, revision, status, updated, true)}}
	}
	older := secret("example.api", "team-a", 3, "superseded", time.Date(2026, 8, 9, 1, 0, 0, 0, time.UTC))
	latest := secret("example.api", "team-a", 4, "deployed", time.Date(2026, 8, 10, 1, 0, 0, 0, time.UTC))
	other := secret("example.api", "team-b", 4, "failed", time.Date(2026, 8, 11, 1, 0, 0, 0, time.UTC))
	releases, failures := ProjectLatestHelmReleaseSecrets([]v1.Secret{older, latest, other, {ObjectMeta: metav1.ObjectMeta{Name: "ordinary", Namespace: "team-a"}, Type: v1.SecretTypeOpaque}})
	if failures != 0 || len(releases) != 2 || releases[0].Namespace != "team-b" || releases[1].Namespace != "team-a" || releases[1].Status != "deployed" {
		t.Fatalf("releases=%#v failures=%d", releases, failures)
	}
}

func TestProjectLatestHelmReleaseSecretsSkipsInvalidAndMismatchedLatest(t *testing.T) {
	valid := v1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "sh.helm.release.v1.valid.v1", Namespace: "team"}, Type: helmReleaseSecretType, Data: map[string][]byte{"release": helmReleasePayloadFixture(t, "valid", "team", 1, "deployed", time.Now(), true)}}
	invalid := v1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "sh.helm.release.v1.invalid.v2", Namespace: "team"}, Type: helmReleaseSecretType, Data: map[string][]byte{"release": []byte("broken")}}
	mismatch := v1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "sh.helm.release.v1.swapped.v1", Namespace: "team"}, Type: helmReleaseSecretType, Data: map[string][]byte{"release": helmReleasePayloadFixture(t, "someone-else", "team", 1, "deployed", time.Now(), true)}}
	releases, failures := ProjectLatestHelmReleaseSecrets([]v1.Secret{valid, invalid, mismatch})
	if failures != 2 || len(releases) != 1 || releases[0].Name != "valid" {
		t.Fatalf("releases=%#v failures=%d", releases, failures)
	}
}

func helmReleasePayloadFixture(t *testing.T, name, namespace string, revision int, status string, updated time.Time, compressed bool) []byte {
	t.Helper()
	stored := map[string]interface{}{
		"name": name, "namespace": namespace, "version": revision,
		"info":     map[string]interface{}{"status": status, "last_deployed": updated, "description": "Ready", "notes": "must not escape"},
		"chart":    map[string]interface{}{"metadata": map[string]interface{}{"name": "example-chart", "version": "1.2.3", "appVersion": "4.5.6"}},
		"manifest": "must not escape", "config": map[string]interface{}{"password": "must not escape"},
	}
	plain, err := json.Marshal(stored)
	if err != nil {
		t.Fatal(err)
	}
	if !compressed {
		return []byte(base64.StdEncoding.EncodeToString(plain))
	}
	var encoded bytes.Buffer
	writer := gzip.NewWriter(&encoded)
	if _, err := writer.Write(plain); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return []byte(base64.StdEncoding.EncodeToString(encoded.Bytes()))
}
