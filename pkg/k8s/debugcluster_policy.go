//go:build debugcluster

package k8s

import (
	"fmt"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// policyObjectsPerNamespace is how many of each configuration and policy kind (service
// accounts, quotas, limit ranges, network policies, autoscalers, disruption budgets, leases) a
// debug namespace holds. A fixed small number: these lists exist to be looked at, not to be
// scaled.
const policyObjectsPerNamespace = 6

// generatePolicyObjects builds the namespaced configuration and policy objects of one debug
// namespace. Values vary with the index so sorting, filtering and search have something to chew on.
func generatePolicyObjects(nsName string) []runtime.Object {
	var out []runtime.Object
	meta := func(kind string, i int) metav1.ObjectMeta {
		name := fmt.Sprintf("debug-%s-%d", kind, i)
		return metav1.ObjectMeta{
			Name:              name,
			Namespace:         nsName,
			UID:               uid(kind, nsName, name),
			ResourceVersion:   nextRV(),
			CreationTimestamp: metav1.Now(),
			Labels:            map[string]string{"app": fmt.Sprintf("app-%d", i%3)},
		}
	}
	for i := range policyObjectsPerNamespace {
		sa := &corev1.ServiceAccount{ObjectMeta: meta("sa", i)}
		for k := 0; k < i%4; k++ {
			sa.Secrets = append(sa.Secrets, corev1.ObjectReference{Name: fmt.Sprintf("%s-token-%d", sa.Name, k)})
		}
		out = append(out, sa)

		out = append(out, &corev1.ResourceQuota{
			ObjectMeta: meta("quota", i),
			Spec: corev1.ResourceQuotaSpec{Hard: corev1.ResourceList{
				corev1.ResourcePods:   resource.MustParse(fmt.Sprintf("%d", 10+i)),
				corev1.ResourceCPU:    resource.MustParse("4"),
				corev1.ResourceMemory: resource.MustParse("8Gi"),
			}},
		})

		out = append(out, &corev1.LimitRange{
			ObjectMeta: meta("limits", i),
			Spec: corev1.LimitRangeSpec{Limits: []corev1.LimitRangeItem{
				{Type: corev1.LimitTypeContainer, Max: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("2")}},
			}},
		})

		np := &networkingv1.NetworkPolicy{ObjectMeta: meta("netpol", i)}
		if i%2 == 0 {
			np.Spec.PodSelector.MatchLabels = map[string]string{"app": fmt.Sprintf("app-%d", i%3)}
		}
		if i%3 == 0 {
			np.Spec.PolicyTypes = []networkingv1.PolicyType{networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress}
		}
		out = append(out, np)

		minReplicas := int32(1 + i%2)
		out = append(out, &autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: meta("hpa", i),
			Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
				ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{APIVersion: "apps/v1", Kind: "Deployment", Name: fmt.Sprintf("debug-deploy-%d", i)},
				MinReplicas:    &minReplicas,
				MaxReplicas:    int32(5 + i),
			},
			Status: autoscalingv2.HorizontalPodAutoscalerStatus{CurrentReplicas: int32(i % 4), DesiredReplicas: int32(1 + i%3)},
		})

		pdb := &policyv1.PodDisruptionBudget{
			ObjectMeta: meta("pdb", i),
			Status:     policyv1.PodDisruptionBudgetStatus{DisruptionsAllowed: int32(i % 3), CurrentHealthy: 3, DesiredHealthy: 2, ExpectedPods: 3},
		}
		if i%2 == 0 {
			v := intstr.FromInt32(int32(1 + i%3))
			pdb.Spec.MinAvailable = &v
		} else {
			v := intstr.FromString("25%")
			pdb.Spec.MaxUnavailable = &v
		}
		out = append(out, pdb)

		holder := fmt.Sprintf("holder-%d_%x", i, 4096+i)
		duration := int32(15 + 15*(i%3))
		renew := metav1.NowMicro()
		out = append(out, &coordinationv1.Lease{
			ObjectMeta: meta("lease", i),
			Spec: coordinationv1.LeaseSpec{
				HolderIdentity: &holder, LeaseDurationSeconds: &duration, RenewTime: &renew,
			},
		})

		// Claims in every phase; a bound claim names its volume and reports a capacity.
		phases := []corev1.PersistentVolumeClaimPhase{corev1.ClaimBound, corev1.ClaimBound, corev1.ClaimPending, corev1.ClaimLost}
		phase := phases[i%len(phases)]
		storageClass := fmt.Sprintf("debug-class-%d", i%3)
		requested := resource.MustParse(fmt.Sprintf("%dGi", 1+i*2))
		pvc := &corev1.PersistentVolumeClaim{
			ObjectMeta: meta("pvc", i),
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				StorageClassName: &storageClass,
				Resources:        corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: requested}},
			},
			Status: corev1.PersistentVolumeClaimStatus{Phase: phase},
		}
		if i%2 == 1 {
			pvc.Spec.AccessModes = append(pvc.Spec.AccessModes, corev1.ReadOnlyMany)
		}
		if phase == corev1.ClaimBound {
			pvc.Spec.VolumeName = fmt.Sprintf("debug-pv-%d", i)
			pvc.Status.Capacity = corev1.ResourceList{corev1.ResourceStorage: requested}
		}
		out = append(out, pvc)
	}
	return out
}
