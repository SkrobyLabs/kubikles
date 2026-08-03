//go:build accelerator_kind

package k8s

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// TestAccelerator00ExactPodLoopbackKind is deliberately isolated behind the
// disposable external gate. It creates only resources in its owned namespace.
func TestAccelerator00ExactPodLoopbackKind(t *testing.T) {
	kubeconfig, image := os.Getenv("KUBECONFIG"), os.Getenv("ACCELERATOR00_KIND_IMAGE")
	if kubeconfig == "" || image == "" {
		t.Fatal("kind gate environment")
	}
	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		t.Fatal("kind configuration")
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		t.Fatal("kind client")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	accelerator00WaitAPIReady(t, ctx, client)
	accelerator00WaitNodesReady(t, ctx, client)
	namespace := os.Getenv("ACCELERATOR00_KIND_NAMESPACE")
	if namespace == "" {
		namespace = fmt.Sprintf("accelerator00-%d", time.Now().UnixNano())
	}
	accelerator00Create(t, ctx, "namespace create", func() error {
		_, createErr := client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}, metav1.CreateOptions{})
		return createErr
	})
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		if err := client.CoreV1().Namespaces().Delete(cleanupCtx, namespace, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			t.Error("namespace delete")
		}
		for {
			_, err := client.CoreV1().Namespaces().Get(cleanupCtx, namespace, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				return
			}
			if err != nil {
				t.Error("namespace cleanup get")
				return
			}
			select {
			case <-cleanupCtx.Done():
				t.Error("namespace cleanup")
				return
			case <-time.After(100 * time.Millisecond):
			}
		}
	})
	accelerator00WaitNamespaceActive(t, ctx, client, namespace)
	target := accelerator00Pod("target", image, "127.0.0.1:8080", "target")
	decoy := accelerator00Pod("decoy", image, "0.0.0.0:8080", "decoy")
	accelerator00Create(t, ctx, "target create", func() error {
		_, createErr := client.CoreV1().Pods(namespace).Create(ctx, target, metav1.CreateOptions{})
		return createErr
	})
	accelerator00Create(t, ctx, "decoy create", func() error {
		_, createErr := client.CoreV1().Pods(namespace).Create(ctx, decoy, metav1.CreateOptions{})
		return createErr
	})
	accelerator00Create(t, ctx, "service create", func() error {
		_, createErr := client.CoreV1().Services(namespace).Create(ctx, &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "decoy"}, Spec: corev1.ServiceSpec{Selector: map[string]string{"accelerator00": "decoy"}, Ports: []corev1.ServicePort{{Port: 8080}}}}, metav1.CreateOptions{})
		return createErr
	})
	target = accelerator00WaitPod(t, ctx, client, namespace, "target")
	_ = accelerator00WaitPod(t, ctx, client, namespace, "decoy")
	accelerator00Probe(t, ctx, client, namespace, image, "target-probe", "http://"+target.Status.PodIP+":8080/id", "target", 3)
	accelerator00Probe(t, ctx, client, namespace, image, "service-probe", "http://decoy:8080/id", "decoy", 0)
	get := func(ctx context.Context, ns, name string) (types.UID, error) {
		pod, err := client.CoreV1().Pods(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return "", err
		}
		return pod.UID, nil
	}
	var factoryCalls atomic.Int32
	factory := func(targetURL *url.URL, addresses, ports []string, stop <-chan struct{}, ready chan struct{}) (baselineForwarder, error) {
		factoryCalls.Add(1)
		return newBaselineClientGoForwarder(cfg, targetURL, 8080, stop, ready)
	}
	forwardCtx, forwardCancel := context.WithCancel(ctx)
	handle, err := baselineExactPodForward(forwardCtx, cfg, namespace, target.Name, target.UID, 8080, get, factory)
	if err != nil || handle.LocalPort() == 0 {
		t.Fatal("exact forward")
	}
	defer handle.Close(context.Background())
	for _, address := range accelerator00NonLoopbackIPv4(t) {
		request, requestErr := accelerator00DirectGet(fmt.Sprintf("http://%s:%d/id", address, handle.LocalPort()))
		if requestErr == nil {
			request.Body.Close()
			t.Fatal("non-loopback listener")
		}
	}
	if response, requestErr := accelerator00DirectGet(fmt.Sprintf("http://[::1]:%d/id", handle.LocalPort())); requestErr == nil {
		response.Body.Close()
		t.Fatal("ipv6 listener")
	}
	response, err := accelerator00DirectGet(fmt.Sprintf("http://127.0.0.1:%d/id", handle.LocalPort()))
	if err != nil {
		t.Fatal("loopback request")
	}
	body, readErr := io.ReadAll(io.LimitReader(response.Body, 128))
	response.Body.Close()
	if readErr != nil || string(body) != "target" {
		t.Fatal("target marker")
	}
	for _, size := range []int{1, 64 * 1024, 1024 * 1024} {
		payload := bytes.Repeat([]byte{'x'}, size)
		response, requestErr := accelerator00DirectClient().Post(fmt.Sprintf("http://127.0.0.1:%d/echo", handle.LocalPort()), "application/octet-stream", bytes.NewReader(payload))
		if requestErr != nil {
			t.Fatal("echo request")
		}
		got, readErr := io.ReadAll(io.LimitReader(response.Body, int64(size+1)))
		response.Body.Close()
		if readErr != nil || !bytes.Equal(got, payload) {
			t.Fatal("echo equality")
		}
	}
	response, err = accelerator00DirectClient().Post(fmt.Sprintf("http://127.0.0.1:%d/echo", handle.LocalPort()), "application/octet-stream", bytes.NewReader(bytes.Repeat([]byte{'x'}, 1024*1024+1)))
	if err != nil {
		t.Fatal("limit request")
	}
	response.Body.Close()
	if response.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatal("limit rejection")
	}
	oldPort := handle.LocalPort()
	forwardCancel()
	select {
	case <-handle.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("caller cancellation")
	}
	if response, requestErr := accelerator00DirectGet(fmt.Sprintf("http://127.0.0.1:%d/id", oldPort)); requestErr == nil {
		response.Body.Close()
		t.Fatal("canceled old port usable")
	}
	// Kubernetes addresses port-forward by Pod name.  Deletion plus a same-name,
	// different-UID replacement must end the old stream; this helper never follows it.
	replacementCtx, replacementCancel := context.WithCancel(ctx)
	replacementHandle, err := baselineExactPodForward(replacementCtx, cfg, namespace, target.Name, target.UID, 8080, get, factory)
	if err != nil {
		t.Fatal("replacement forward")
	}
	originalUID := target.UID
	replacementPort := replacementHandle.LocalPort()
	replacementCloseCtx, replacementCloseCancel := context.WithTimeout(ctx, 15*time.Second)
	defer replacementCloseCancel()
	if err := client.CoreV1().Pods(namespace).Delete(replacementCloseCtx, target.Name, metav1.DeleteOptions{}); err != nil {
		t.Fatal("target delete")
	}
	for {
		_, err := client.CoreV1().Pods(namespace).Get(replacementCloseCtx, target.Name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			break
		}
		if err != nil && !accelerator00TransientSetupError(err) {
			t.Fatal("target deletion")
		}
		select {
		case <-replacementCloseCtx.Done():
			t.Fatal("target deletion timeout")
		case <-time.After(100 * time.Millisecond):
		}
	}
	accelerator00Create(t, replacementCloseCtx, "replacement create", func() error {
		_, createErr := client.CoreV1().Pods(namespace).Create(replacementCloseCtx, accelerator00Pod("target", image, "127.0.0.1:8080", "replacement"), metav1.CreateOptions{})
		return createErr
	})
	target = accelerator00WaitPod(t, replacementCloseCtx, client, namespace, "target")
	if target.UID == originalUID {
		t.Fatal("replacement uid")
	}
	// client-go keeps an idle upgraded connection open until it observes a
	// stream failure. Exercise only the old listener after the different-UID
	// replacement is ready: every request must fail rather than follow the name,
	// and loss detection must close the old forward within the fixed bound.
	accelerator00WaitForwardClosed(t, replacementCloseCtx, replacementHandle, replacementPort, "replacement old port usable", "replacement termination")
	if replacementHandle.Reason() != baselineRemoteClosed || replacementHandle.LocalPort() != 0 {
		t.Fatal("replacement reason")
	}
	if response, requestErr := accelerator00DirectGet(fmt.Sprintf("http://127.0.0.1:%d/id", replacementPort)); requestErr == nil {
		response.Body.Close()
		t.Fatal("replacement old port usable")
	}
	if factoryCalls.Load() != 2 {
		t.Fatal("replacement followed")
	}
	replacementCancel()

	remoteCtx, remoteCancel := context.WithCancel(ctx)
	remoteHandle, err := baselineExactPodForward(remoteCtx, cfg, namespace, target.Name, target.UID, 8080, get, factory)
	if err != nil {
		t.Fatal("remote forward")
	}
	remotePort := remoteHandle.LocalPort()
	response, err = accelerator00DirectClient().Post(fmt.Sprintf("http://127.0.0.1:%d/terminate", remotePort), "text/plain", nil)
	if err != nil || response.StatusCode != http.StatusNoContent {
		t.Fatal("remote terminate")
	}
	response.Body.Close()
	remoteCloseCtx, remoteCloseCancel := context.WithTimeout(ctx, 15*time.Second)
	defer remoteCloseCancel()
	accelerator00WaitPodStopped(t, remoteCloseCtx, client, namespace, target.Name)
	// As with replacement, exercise the listener after the workload has
	// stopped so the otherwise-idle upgraded connection observes the loss.
	accelerator00WaitForwardClosed(t, remoteCloseCtx, remoteHandle, remotePort, "remote old port usable", "remote closure")
	if remoteHandle.Reason() != baselineRemoteClosed || remoteHandle.LocalPort() != 0 {
		t.Fatal("remote reason")
	}
	if response, requestErr := accelerator00DirectGet(fmt.Sprintf("http://127.0.0.1:%d/id", remotePort)); requestErr == nil {
		response.Body.Close()
		t.Fatal("remote old port usable")
	}
	if factoryCalls.Load() != 3 {
		t.Fatal("remote retry")
	}
	remoteCancel()
}

func accelerator00Pod(name, image, address, marker string) *corev1.Pod {
	return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{"accelerator00": name}}, Spec: corev1.PodSpec{RestartPolicy: corev1.RestartPolicyNever, Containers: []corev1.Container{{Name: "echo", Image: image, ImagePullPolicy: corev1.PullNever, Args: []string{"-mode=serve", "-address=" + address, "-marker=" + marker}, ReadinessProbe: &corev1.Probe{ProbeHandler: corev1.ProbeHandler{Exec: &corev1.ExecAction{Command: []string{"/accelerator00-echo", "-mode=probe", "-url=http://127.0.0.1:8080/id", "-marker=" + marker}}}}}}}}
}

func accelerator00WaitAPIReady(t *testing.T, ctx context.Context, client kubernetes.Interface) {
	t.Helper()
	for {
		err := client.Discovery().RESTClient().Get().AbsPath("/readyz").Do(ctx).Error()
		if err == nil {
			return
		}
		if !accelerator00TransientSetupError(err) && !apierrors.IsInternalError(err) {
			t.Fatal("api readiness")
		}
		accelerator00WaitSetupRetry(t, ctx, "api readiness")
	}
}

func accelerator00WaitNodesReady(t *testing.T, ctx context.Context, client kubernetes.Interface) {
	t.Helper()
	for {
		nodes, err := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
		if err != nil {
			if !accelerator00TransientSetupError(err) {
				t.Fatal("node readiness")
			}
		} else if len(nodes.Items) > 0 {
			allReady := true
			for _, node := range nodes.Items {
				ready := false
				for _, condition := range node.Status.Conditions {
					if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
						ready = true
						break
					}
				}
				allReady = allReady && ready
			}
			if allReady {
				return
			}
		}
		accelerator00WaitSetupRetry(t, ctx, "node readiness")
	}
}

func accelerator00WaitNamespaceActive(t *testing.T, ctx context.Context, client kubernetes.Interface, namespace string) {
	t.Helper()
	for {
		current, err := client.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
		if err != nil {
			if !apierrors.IsNotFound(err) && !accelerator00TransientSetupError(err) {
				t.Fatal("namespace readiness")
			}
		} else {
			switch current.Status.Phase {
			case corev1.NamespaceActive:
				return
			case corev1.NamespaceTerminating:
				t.Fatal("namespace readiness")
			}
		}
		accelerator00WaitSetupRetry(t, ctx, "namespace readiness")
	}
}

func accelerator00Create(t *testing.T, ctx context.Context, failure string, create func() error) {
	t.Helper()
	for {
		err := create()
		if err == nil {
			return
		}
		if !accelerator00TransientSetupError(err) {
			t.Fatal(failure)
		}
		accelerator00WaitSetupRetry(t, ctx, failure)
	}
}

func accelerator00TransientSetupError(err error) bool {
	if apierrors.IsTimeout(err) || apierrors.IsServerTimeout(err) || apierrors.IsTooManyRequests(err) || apierrors.IsServiceUnavailable(err) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	return errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.EHOSTUNREACH) || errors.Is(err, syscall.ENETUNREACH)
}

func TestAccelerator00TransientSetupClassification(t *testing.T) {
	for _, err := range []error{
		apierrors.NewTimeoutError("fixed", 0),
		apierrors.NewTooManyRequests("fixed", 0),
		apierrors.NewServiceUnavailable("fixed"),
		&net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED},
		&net.OpError{Op: "dial", Net: "tcp", Err: syscall.EHOSTUNREACH},
		&net.OpError{Op: "dial", Net: "tcp", Err: syscall.ENETUNREACH},
	} {
		if !accelerator00TransientSetupError(err) {
			t.Fatal("transient setup classification")
		}
	}
	for _, err := range []error{
		apierrors.NewUnauthorized("fixed"),
		apierrors.NewInternalError(errors.New("fixed")),
		errors.New("fixed"),
	} {
		if accelerator00TransientSetupError(err) {
			t.Fatal("nontransient setup classification")
		}
	}
}

func accelerator00WaitSetupRetry(t *testing.T, ctx context.Context, failure string) {
	t.Helper()
	select {
	case <-ctx.Done():
		t.Fatal(failure)
	case <-time.After(100 * time.Millisecond):
	}
}

func accelerator00WaitPod(t *testing.T, ctx context.Context, client kubernetes.Interface, namespace, name string) *corev1.Pod {
	t.Helper()
	for {
		pod, err := client.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil && !apierrors.IsNotFound(err) && !accelerator00TransientSetupError(err) {
			t.Fatal("pod ready")
		}
		if err == nil && pod.Status.Phase == corev1.PodRunning {
			for _, condition := range pod.Status.Conditions {
				if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
					return pod
				}
			}
		}
		select {
		case <-ctx.Done():
			t.Fatal("pod ready")
			return nil
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func accelerator00WaitPodStopped(t *testing.T, ctx context.Context, client kubernetes.Interface, namespace, name string) {
	t.Helper()
	for {
		pod, err := client.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil && !accelerator00TransientSetupError(err) {
			t.Fatal("remote pod get")
		}
		if err == nil && (pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed) {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("remote pod stop")
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func accelerator00WaitForwardClosed(t *testing.T, ctx context.Context, handle *baselineExactPodHandle, port uint16, usableFailure, timeoutFailure string) {
	t.Helper()
	for {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/id", port), nil)
		if err != nil {
			t.Fatal("old port request")
		}
		if response, err := accelerator00DirectClient().Do(request); err == nil {
			response.Body.Close()
			t.Fatal(usableFailure)
		}
		select {
		case <-handle.Done():
			return
		case <-ctx.Done():
			t.Fatal(timeoutFailure)
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func accelerator00Probe(t *testing.T, ctx context.Context, client kubernetes.Interface, namespace, image, name, endpoint, marker string, exitCode int32) {
	t.Helper()
	pod := accelerator00Pod(name, image, "0.0.0.0:8080", "unused")
	pod.Spec.Containers[0].Args = []string{"-mode=probe", "-url=" + endpoint, "-marker=" + marker}
	accelerator00Create(t, ctx, "probe create", func() error {
		_, createErr := client.CoreV1().Pods(namespace).Create(ctx, pod, metav1.CreateOptions{})
		return createErr
	})
	for {
		current, err := client.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil && !apierrors.IsNotFound(err) && !accelerator00TransientSetupError(err) {
			t.Fatal("probe get")
		}
		if err == nil && current.Status.Phase == corev1.PodSucceeded {
			if exitCode == 0 {
				return
			}
			t.Fatal("probe unexpectedly succeeded")
		}
		if err == nil && current.Status.Phase == corev1.PodFailed {
			if len(current.Status.ContainerStatuses) != 1 || current.Status.ContainerStatuses[0].State.Terminated == nil || current.Status.ContainerStatuses[0].State.Terminated.ExitCode != exitCode {
				t.Fatal("probe exit code")
			}
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("probe timeout")
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func accelerator00DirectGet(target string) (*http.Response, error) {
	return accelerator00DirectClient().Get(target)
}
func accelerator00DirectClient() *http.Client {
	return &http.Client{Transport: &http.Transport{Proxy: nil, DialContext: (&net.Dialer{}).DialContext}}
}

func accelerator00NonLoopbackIPv4(t *testing.T) []string {
	t.Helper()
	interfaces, err := net.Interfaces()
	if err != nil {
		t.Fatal("interfaces")
	}
	var result []string
	for _, iface := range interfaces {
		addrs, err := iface.Addrs()
		if err != nil {
			t.Fatal("interface addresses")
		}
		for _, addr := range addrs {
			ip, _, err := net.ParseCIDR(addr.String())
			if err == nil && ip.To4() != nil && !ip.IsLoopback() {
				result = append(result, ip.String())
			}
		}
	}
	if len(result) == 0 {
		t.Fatal("no non-loopback ipv4")
	}
	return result
}
