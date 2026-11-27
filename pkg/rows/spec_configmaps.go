package rows

import (
	"encoding/base64"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"kubikles/pkg/store"
)

func init() { MustRegister(configMapsSpec()) }

// systemConfigMapNames are auto-created in every namespace (SYSTEM_CONFIGMAP_NAMES in
// ConfigMapList.tsx); the "system" preset hides them.
var systemConfigMapNames = map[string]bool{"kube-root-ca.crt": true}

func configMapObj(r *Row) *corev1.ConfigMap {
	cm, _ := r.E.Obj.(*corev1.ConfigMap)
	return cm
}

func configMapKeys(r *Row) []string {
	if cm := configMapObj(r); cm != nil {
		return jsKeys(cm.Data)
	}
	return nil
}

func configMapBinaryKeys(r *Row) []string {
	if cm := configMapObj(r); cm != nil {
		return jsKeys(cm.BinaryData)
	}
	return nil
}

// binaryValueLen is the stringified length of a binaryData value as the browser sees it: the
// base64 text (encoding/json writes []byte as base64), or null for a nil slice.
func binaryValueLen(b []byte) int {
	if b == nil {
		return 4
	}
	return jsStringLen(base64.StdEncoding.EncodeToString(b))
}

// configMapSize is JSON.stringify(data || {}).length + JSON.stringify(binaryData || {}).length.
func configMapSize(r *Row) int {
	cm := configMapObj(r)
	if cm == nil {
		return 4
	}
	return jsStringifyMapLen(cm.Data, jsStringLen) + jsStringifyMapLen(cm.BinaryData, binaryValueLen)
}

func configMapsSpec() *Spec {
	return &Spec{
		Type:       "configmaps",
		Namespaced: true,
		// The keys, keycount and binarykeys search fields read data and binaryData, which the
		// Table representation does not carry, so this type stays on full objects.
		Needs: store.ModeFull,
		Columns: []Column{
			nameColumn(),
			namespaceColumn(),
			{Key: "keys", Kind: KindString, CellType: "string", FilterType: FilterSelect,
				Cell: func(r *Row) interface{} { return strings.Join(configMapKeys(r), ", ") }},
			ageColumn(creationAge),
			{Key: "keyCount", Kind: KindNumber, CellType: "number", FilterType: FilterSelect,
				Cell: func(r *Row) interface{} { return numberCell(len(configMapKeys(r))) }},
			{Key: "binaryKeys", Kind: KindNumber, CellType: "number", FilterType: FilterSelect,
				Cell: func(r *Row) interface{} { return numberCell(len(configMapBinaryKeys(r))) }},
			{Key: "size", Kind: KindNumber, CellType: "number", FilterType: FilterSelect,
				Cell: func(r *Row) interface{} { return numberCell(configMapSize(r)) }},
		},
		Search: append(commonSearchFields(),
			SearchField{Name: "keys", Aliases: []string{"key", "datakeys"}, Extract: func(r *Row) string {
				return strings.Join(configMapKeys(r), " ")
			}},
			SearchField{Name: "keycount", Aliases: []string{"count"}, Extract: func(r *Row) string {
				return itoa(len(configMapKeys(r)))
			}},
			SearchField{Name: "binarykeys", Aliases: []string{"binarykey", "binary"}, Extract: func(r *Row) string {
				return strings.Join(configMapBinaryKeys(r), " ")
			}},
		),
		Presets: []Preset{{Name: "system", Hide: func(r *Row) bool { return systemConfigMapNames[r.Name()] }}},
	}
}
