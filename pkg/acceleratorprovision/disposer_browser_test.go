package acceleratorprovision

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"
	"kubikles/pkg/agent"
	"kubikles/pkg/server"
)

func TestDetachAndDisposeNowLinearizeWithoutTransportLeak(t *testing.T) {
	for iteration := 0; iteration < 50; iteration++ {
		workload := connectorWorkload(t)
		socketOrder, tunnelOrder := []string{}, []string{}
		socket := newRecordedSessionSocket(&socketOrder)
		activeTunnel := newRecordedTunnel(&tunnelOrder)
		session := newConnectedSession(workload.connectorState.receipt, authenticatedTestInfo(), connectedIdentity{sessionID: "session-a", instanceID: "instance-a", generation: 1}, socket, activeTunnel, processResumeClock{})
		if !workload.publishCurrentSession(session) {
			t.Fatalf("iteration %d session publication rejected", iteration)
		}
		service := &DisposalService{phaseContext: context.WithTimeout}
		start := make(chan struct{})
		detached := make(chan bool, 1)
		disposed := make(chan DisposalResult, 1)
		go func() {
			<-start
			_, ok := session.detachCreatorForBrowser(context.Background())
			detached <- ok
		}()
		go func() {
			<-start
			disposed <- service.DisposeNow(context.Background(), workload)
		}()
		close(start)
		_ = <-detached
		result := <-disposed
		if result.Effective != DisposalImmediate || !tunnelEnded(activeTunnel) {
			t.Fatalf("iteration %d disposal=%#v tunnelEnded=%v", iteration, result, tunnelEnded(activeTunnel))
		}
		workload.connectorState.mu.Lock()
		current, browser, owned := workload.connectorState.currentSession, workload.connectorState.browserTransport, workload.connectorState.browserOwned
		workload.connectorState.mu.Unlock()
		if current != nil || browser != nil || owned {
			t.Fatalf("iteration %d retained current=%v browser=%v owned=%v", iteration, current != nil, browser != nil, owned)
		}
	}
}

func TestBrowserEscalationSynchronouslyFencesRetainedTransport(t *testing.T) {
	workload, owned, activeTunnel := detachedBrowserFixture(t)
	observerEntered, releaseObserver := make(chan struct{}), make(chan struct{})
	service := &DisposalService{
		observeBrowser: func(context.Context, *workloadReceipt, <-chan struct{}) DrainObservationStatus {
			close(observerEntered)
			<-releaseObserver
			return DrainEscalated
		},
		phaseContext: context.WithTimeout,
	}
	browserDone := make(chan struct{})
	go func() {
		_ = service.DisposeAfterBrowser(context.Background(), owned)
		close(browserDone)
	}()
	<-observerEntered
	completion := service.startDisposeNow(context.Background(), workload)
	completion.waitTransportFenced()
	if !tunnelEnded(activeTunnel) {
		t.Fatal("escalation returned before retained transport Stop fence")
	}
	select {
	case <-browserDone:
		t.Fatal("blocked observer unexpectedly completed")
	default:
	}
	close(releaseObserver)
	select {
	case <-browserDone:
	case <-time.After(time.Second):
		t.Fatal("browser disposal did not join after observer release")
	}
}

func detachedBrowserFixture(t *testing.T) (*ProvisionedWorkload, *browserOwnedWorkload, *recordedTunnel) {
	t.Helper()
	workload := connectorWorkload(t)
	order := []string{}
	socket := newRecordedSessionSocket(&order)
	activeTunnel := newRecordedTunnel(&order)
	session := newConnectedSession(workload.connectorState.receipt, authenticatedTestInfo(), connectedIdentity{sessionID: "session-a", instanceID: "instance-a", generation: 1}, socket, activeTunnel, processResumeClock{})
	if !workload.publishCurrentSession(session) {
		t.Fatal("session publication rejected")
	}
	owned, ok := session.detachCreatorForBrowser(context.Background())
	if !ok || owned == nil {
		t.Fatal("browser handoff rejected")
	}
	return workload, owned, activeTunnel
}

func authenticatedTestInfo() server.AuthenticatedAcceleratorInfo {
	return server.AuthenticatedAcceleratorInfo{Capabilities: agent.V1Capabilities()}
}

func TestDisposeAfterBrowserWaitsForExactTerminalWorkload(t *testing.T) {
	workload, owned, activeTunnel := detachedBrowserFixture(t)
	client := workload.snapshot.Clientset()
	var cleanupCalls atomic.Int32
	service := &DisposalService{
		observeBrowser: observeExactBrowserTerminal,
		cleanupOwned: func(context.Context, *workloadReceipt) (OwnershipStatus, UninstallStatus, DisappearanceStatus) {
			cleanupCalls.Add(1)
			return OwnershipProven, UninstallSucceeded, DisappearanceSucceeded
		},
		phaseContext: context.WithTimeout,
	}
	result := make(chan DisposalResult, 1)
	go func() { result <- service.DisposeAfterBrowser(context.Background(), owned) }()
	select {
	case got := <-result:
		t.Fatalf("running job disposed early: %#v", got)
	case <-time.After(30 * time.Millisecond):
	}
	if tunnelEnded(activeTunnel) || cleanupCalls.Load() != 0 {
		t.Fatal("normal browser observation closed transport or cleaned early")
	}
	job, err := client.BatchV1().Jobs(workload.ReleaseNamespace).Get(context.Background(), workload.Job.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	job.Status.Conditions = []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionTrue}}
	if _, err = client.BatchV1().Jobs(workload.ReleaseNamespace).UpdateStatus(context.Background(), job, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-result:
		if got.Observation != DrainComplete || got.Ownership != OwnershipProven || got.Uninstall != UninstallSucceeded {
			t.Fatalf("result=%#v", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("terminal job did not dispose")
	}
	if !tunnelEnded(activeTunnel) || cleanupCalls.Load() != 1 {
		t.Fatalf("tunnelEnded=%v cleanup=%d", tunnelEnded(activeTunnel), cleanupCalls.Load())
	}
}

func TestBrowserTerminalObservationPrecedesUninstall(t *testing.T) {
	workload, owned, _ := detachedBrowserFixture(t)
	client := workload.snapshot.Clientset()
	exact := make(chan struct{})
	var order []string
	service := &DisposalService{
		observeBrowser: func(ctx context.Context, receipt *workloadReceipt, force <-chan struct{}) DrainObservationStatus {
			status := observeExactBrowserTerminalWithHooks(ctx, receipt, force, func() { close(exact) }, nil)
			order = append(order, "terminal")
			return status
		},
		cleanupOwned: func(context.Context, *workloadReceipt) (OwnershipStatus, UninstallStatus, DisappearanceStatus) {
			order = append(order, "uninstall")
			return OwnershipProven, UninstallSucceeded, DisappearanceSucceeded
		},
		phaseContext: context.WithTimeout,
	}
	result := make(chan DisposalResult, 1)
	go func() { result <- service.DisposeAfterBrowser(context.Background(), owned) }()
	<-exact
	if len(order) != 0 {
		t.Fatalf("cleanup ran before terminal observation: %v", order)
	}
	job, err := client.BatchV1().Jobs(workload.ReleaseNamespace).Get(context.Background(), workload.Job.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	job.Status.Conditions = []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionTrue}}
	if _, err = client.BatchV1().Jobs(workload.ReleaseNamespace).UpdateStatus(context.Background(), job, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-result:
		if got.Observation != DrainComplete || got.Uninstall != UninstallSucceeded {
			t.Fatalf("result=%#v", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("terminal Browser workload did not uninstall")
	}
	if len(order) != 2 || order[0] != "terminal" || order[1] != "uninstall" {
		t.Fatalf("terminal/uninstall order=%v", order)
	}
}

func TestBrowserDisposalEscalatesWithoutSecondOperation(t *testing.T) {
	workload, owned, activeTunnel := detachedBrowserFixture(t)
	var cleanupCalls atomic.Int32
	service := &DisposalService{
		observeBrowser: observeExactBrowserTerminal,
		cleanupOwned: func(context.Context, *workloadReceipt) (OwnershipStatus, UninstallStatus, DisappearanceStatus) {
			cleanupCalls.Add(1)
			return OwnershipProven, UninstallSucceeded, DisappearanceSucceeded
		},
		phaseContext: context.WithTimeout,
	}
	browserResult := make(chan DisposalResult, 1)
	go func() { browserResult <- service.DisposeAfterBrowser(context.Background(), owned) }()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		workload.connectorState.mu.Lock()
		started := workload.connectorState.disposal != nil
		workload.connectorState.mu.Unlock()
		if started {
			break
		}
		time.Sleep(time.Millisecond)
	}
	now := service.DisposeNow(context.Background(), workload)
	first := <-browserResult
	if now.Effective != DisposalImmediateEscalation || first.Effective != DisposalImmediateEscalation || now != first {
		t.Fatalf("browser=%#v now=%#v", first, now)
	}
	if !tunnelEnded(activeTunnel) || cleanupCalls.Load() != 1 {
		t.Fatalf("tunnelEnded=%v cleanup=%d", tunnelEnded(activeTunnel), cleanupCalls.Load())
	}
}

func TestBrowserTerminalObserverRejectsReplacementUntilEscalated(t *testing.T) {
	workload := connectorWorkload(t)
	receipt := workload.connectorState.receipt
	client := workload.snapshot.Clientset()
	job, err := client.BatchV1().Jobs(workload.ReleaseNamespace).Get(context.Background(), workload.Job.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	job.UID = "replacement"
	if _, err = client.BatchV1().Jobs(workload.ReleaseNamespace).Update(context.Background(), job, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	force := make(chan struct{})
	result := make(chan DrainObservationStatus, 1)
	ambiguous := make(chan struct{})
	go func() {
		result <- observeExactBrowserTerminalWithHooks(context.Background(), receipt, force, nil, func() { close(ambiguous) })
	}()
	<-ambiguous
	if err := client.BatchV1().Jobs(workload.ReleaseNamespace).Delete(context.Background(), workload.Job.Name, metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := client.CoreV1().Pods(workload.ReleaseNamespace).Delete(context.Background(), workload.Pod.Name, metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-result:
		t.Fatalf("transient replacement disappearance authorized cleanup: %s", got)
	default:
	}
	close(force)
	if got := <-result; got != DrainEscalated {
		t.Fatalf("escalation=%s", got)
	}
}

func TestBrowserTerminalObserverRejectsCorrelatedRunningReplacement(t *testing.T) {
	workload := connectorWorkload(t)
	receipt := workload.connectorState.receipt
	client := workload.snapshot.Clientset()
	force := make(chan struct{})
	result := make(chan DrainObservationStatus, 1)
	exact, ambiguous := make(chan struct{}), make(chan struct{})
	go func() {
		result <- observeExactBrowserTerminalWithHooks(context.Background(), receipt, force, func() { close(exact) }, func() { close(ambiguous) })
	}()
	<-exact
	if err := client.BatchV1().Jobs(workload.ReleaseNamespace).Delete(context.Background(), workload.Job.Name, metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := client.CoreV1().Pods(workload.ReleaseNamespace).Delete(context.Background(), workload.Pod.Name, metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	replacement := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name: "replacement", Namespace: workload.ReleaseNamespace, UID: "replacement",
		Labels: map[string]string{"kubikles.io/workload-session-id": receipt.workloadSessionID, "app.kubernetes.io/instance": receipt.releaseName, "app.kubernetes.io/managed-by": "Helm"},
	}}
	if _, err := client.CoreV1().Pods(workload.ReleaseNamespace).Create(context.Background(), replacement, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	<-ambiguous
	if err := client.CoreV1().Pods(workload.ReleaseNamespace).Delete(context.Background(), replacement.Name, metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-result:
		t.Fatalf("correlation ambiguity authorized cleanup: %s", got)
	default:
	}
	close(force)
	if got := <-result; got != DrainEscalated {
		t.Fatalf("escalation=%s", got)
	}
}

func TestBrowserTerminalObserverFailedTTLAndTransientRead(t *testing.T) {
	t.Run("failed", func(t *testing.T) {
		workload := connectorWorkload(t)
		client := workload.snapshot.Clientset()
		job, err := client.BatchV1().Jobs(workload.ReleaseNamespace).Get(context.Background(), workload.Job.Name, metav1.GetOptions{})
		if err != nil {
			t.Fatal(err)
		}
		job.Status.Conditions = []batchv1.JobCondition{{Type: batchv1.JobFailed, Status: corev1.ConditionTrue}}
		if _, err = client.BatchV1().Jobs(workload.ReleaseNamespace).UpdateStatus(context.Background(), job, metav1.UpdateOptions{}); err != nil {
			t.Fatal(err)
		}
		if got := observeExactBrowserTerminal(context.Background(), workload.connectorState.receipt, make(chan struct{})); got != DrainFailed {
			t.Fatalf("failed observation=%s", got)
		}
	})

	t.Run("ttl disappearance", func(t *testing.T) {
		workload := connectorWorkload(t)
		client := workload.snapshot.Clientset()
		exact := make(chan struct{})
		result := make(chan DrainObservationStatus, 1)
		go func() {
			result <- observeExactBrowserTerminalWithHooks(context.Background(), workload.connectorState.receipt, make(chan struct{}), func() { close(exact) }, nil)
		}()
		<-exact
		if err := client.BatchV1().Jobs(workload.ReleaseNamespace).Delete(context.Background(), workload.Job.Name, metav1.DeleteOptions{}); err != nil {
			t.Fatal(err)
		}
		if err := client.CoreV1().Pods(workload.ReleaseNamespace).Delete(context.Background(), workload.Pod.Name, metav1.DeleteOptions{}); err != nil {
			t.Fatal(err)
		}
		select {
		case got := <-result:
			if got != DrainNotFound {
				t.Fatalf("TTL observation=%s", got)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("TTL disappearance was not observed")
		}
	})

	t.Run("transient job read", func(t *testing.T) {
		workload := connectorWorkload(t)
		client := workload.snapshot.Clientset()
		job, err := client.BatchV1().Jobs(workload.ReleaseNamespace).Get(context.Background(), workload.Job.Name, metav1.GetOptions{})
		if err != nil {
			t.Fatal(err)
		}
		job.Status.Conditions = []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionTrue}}
		if _, err = client.BatchV1().Jobs(workload.ReleaseNamespace).UpdateStatus(context.Background(), job, metav1.UpdateOptions{}); err != nil {
			t.Fatal(err)
		}
		reactor, ok := client.(interface {
			PrependReactor(string, string, k8stesting.ReactionFunc)
		})
		if !ok {
			t.Fatal("fake client has no reactor seam")
		}
		var reads atomic.Int32
		reactor.PrependReactor("get", "jobs", func(k8stesting.Action) (bool, runtime.Object, error) {
			if reads.Add(1) == 1 {
				return true, nil, errors.New("transient browser job read")
			}
			return false, nil, nil
		})
		if got := observeExactBrowserTerminal(context.Background(), workload.connectorState.receipt, make(chan struct{})); got != DrainComplete || reads.Load() < 2 {
			t.Fatalf("transient observation=%s reads=%d", got, reads.Load())
		}
	})
}
