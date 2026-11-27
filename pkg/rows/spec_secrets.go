package rows

import (
	"encoding/json"

	corev1 "k8s.io/api/core/v1"

	"kubikles/pkg/store"
)

func init() { MustRegister(secretsSpec()) }

// HelmReleaseSecretType marks the secrets Helm stores release state in.
const HelmReleaseSecretType = "helm.sh/release.v1"

// secretInfo reads a secret's type and number of data keys from whichever representation the
// shard holds: the server-printed Table cells (Type, Data) or a full object when the Table
// shard degraded (fake clientsets, aggregated APIs).
func secretInfo(r *Row) (typ string, dataKeys int) {
	if t, ok := r.TableCell("Type"); ok {
		typ, _ = t.(string)
		if d, ok := r.TableCell("Data"); ok {
			dataKeys = intCell(d)
		}
		return typ, dataKeys
	}
	if s, ok := r.E.Obj.(*corev1.Secret); ok {
		return string(s.Type), len(s.Data)
	}
	return "", 0
}

func intCell(v interface{}) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int64:
		return int(n)
	case int:
		return n
	case int32:
		return int(n)
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	}
	return 0
}

func secretsSpec() *Spec {
	return &Spec{
		Type:       "secrets",
		Namespaced: true,
		// Table mode: the list never sees secret data, only its type and key count.
		Needs: store.ModeTable,
		Columns: []Column{
			nameColumn(),
			namespaceColumn(),
			{Key: "type", Kind: KindString, CellType: "string | null", FilterType: FilterSelect,
				Cell: func(r *Row) interface{} {
					t, _ := secretInfo(r)
					return stringOrNil(t)
				}},
			ageColumn(creationAge),
			// The legacy keys column joined key names, which list items never carried (it
			// only had values on objects that arrived through a watch event). The view shows the
			// count; key names need the object. The legacy size column is dropped.
			{Key: "keys", Kind: KindNumber, CellType: "number", FilterType: FilterNone,
				Cell: func(r *Row) interface{} {
					_, n := secretInfo(r)
					return numberCell(n)
				}},
		},
		Search: append(commonSearchFields(),
			SearchField{Name: "type", Aliases: []string{"secrettype"}, Extract: func(r *Row) string {
				if t, _ := secretInfo(r); t != "" {
					return t
				}
				return "Opaque"
			}},
			SearchField{Name: "keycount", Aliases: []string{"count", "datakeys"}, Extract: func(r *Row) string {
				_, n := secretInfo(r)
				return itoa(n)
			}},
		),
		Presets: []Preset{{Name: "helm", Hide: func(r *Row) bool {
			t, _ := secretInfo(r)
			return t == HelmReleaseSecretType
		}}},
		Notes: "Intentional difference from the legacy list: keys is the number of data keys instead of the joined key names, " +
			"and the size column is dropped. Secret values never leave the API server for the list, and key names are " +
			"shown after fetching the object on demand.",
	}
}
