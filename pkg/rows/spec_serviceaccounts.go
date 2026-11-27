package rows

import (
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"kubikles/pkg/store"
)

func init() { MustRegister(serviceAccountsSpec()) }

func saObj(r *Row) *corev1.ServiceAccount {
	s, _ := r.E.Obj.(*corev1.ServiceAccount)
	return s
}

func saSecretCount(r *Row) int {
	if s := saObj(r); s != nil {
		return len(s.Secrets)
	}
	return 0
}

func serviceAccountsSpec() *Spec {
	return &Spec{
		Type:       "serviceaccounts",
		Namespaced: true,
		Needs:      store.ModeFull, // secrets and pull secrets are not metadata
		Columns: []Column{
			nameColumn(),
			namespaceColumn(),
			{Key: "secrets", Kind: KindNumber, CellType: "number", FilterType: FilterSelect,
				Cell: func(r *Row) interface{} { return numberCell(saSecretCount(r)) }},
			ageColumn(creationAge),
		},
		Search: append(commonSearchFields(),
			SearchField{Name: "secrets", Aliases: []string{"secret"}, Extract: func(r *Row) string {
				s := saObj(r)
				if s == nil {
					return ""
				}
				names := make([]string, len(s.Secrets))
				for i, ref := range s.Secrets {
					names[i] = ref.Name
				}
				return strings.Join(names, " ")
			}},
			SearchField{Name: "secretcount", Aliases: []string{"secretscount", "numsecrets"}, Extract: func(r *Row) string { return itoa(saSecretCount(r)) }},
			SearchField{Name: "imagepullsecrets", Aliases: []string{"pullsecrets", "imagepull"}, Extract: func(r *Row) string {
				s := saObj(r)
				if s == nil {
					return ""
				}
				names := make([]string, len(s.ImagePullSecrets))
				for i, ref := range s.ImagePullSecrets {
					names[i] = ref.Name
				}
				return strings.Join(names, " ")
			}},
			SearchField{Name: "automount", Aliases: []string{"automounttoken", "automountserviceaccounttoken"}, Extract: func(r *Row) string {
				if s := saObj(r); s != nil && s.AutomountServiceAccountToken != nil {
					return strconv.FormatBool(*s.AutomountServiceAccountToken)
				}
				return "true"
			}},
		),
	}
}
