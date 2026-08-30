package assertions

import (
	"context"
	"strings"
	"testing"

	cnpgv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	preview "github.com/documentdb/documentdb-operator/api/preview"
	shareddb "github.com/documentdb/documentdb-operator/test/shared/documentdb"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s, err := shareddb.NewScheme(corev1.AddToScheme, cnpgv1.AddToScheme)
	if err != nil {
		t.Fatalf("NewScheme: %v", err)
	}
	return s
}

func TestAssertDocumentDBReady(t *testing.T) {
	t.Parallel()
	s := newScheme(t)
	dd := &preview.DocumentDB{
		ObjectMeta: metav1.ObjectMeta{Name: "db1", Namespace: "ns"},
		Status:     preview.DocumentDBStatus{Status: "Cluster in healthy state"},
	}
	notReady := &preview.DocumentDB{
		ObjectMeta: metav1.ObjectMeta{Name: "db2", Namespace: "ns"},
		Status:     preview.DocumentDBStatus{Status: "Setting up primary"},
	}
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(dd, notReady).Build()

	if err := AssertDocumentDBReady(context.Background(), c, client.ObjectKey{Namespace: "ns", Name: "db1"})(); err != nil {
		t.Fatalf("expected ready, got err=%v", err)
	}
	if err := AssertDocumentDBReady(context.Background(), c, client.ObjectKey{Namespace: "ns", Name: "db2"})(); err == nil {
		t.Fatalf("expected not-ready error")
	}
	if err := AssertDocumentDBReady(context.Background(), c, client.ObjectKey{Namespace: "ns", Name: "missing"})(); err == nil {
		t.Fatalf("expected error for missing object")
	}
}

func TestAssertInstanceCount(t *testing.T) {
	t.Parallel()
	s := newScheme(t)
	dd := &preview.DocumentDB{ObjectMeta: metav1.ObjectMeta{Name: "db", Namespace: "ns"}}
	cluster := &cnpgv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: "db", Namespace: "ns"},
		Status:     cnpgv1.ClusterStatus{ReadyInstances: 3},
	}
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(dd, cluster).Build()
	key := client.ObjectKey{Namespace: "ns", Name: "db"}

	if err := AssertInstanceCount(context.Background(), c, key, 3)(); err != nil {
		t.Fatalf("want ok, got %v", err)
	}
	if err := AssertInstanceCount(context.Background(), c, key, 2)(); err == nil {
		t.Fatalf("want mismatch error")
	}
}

func TestAssertPVCCount(t *testing.T) {
	t.Parallel()
	s := newScheme(t)
	pvcs := []client.Object{
		&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{
			Name: "p1", Namespace: "ns", Labels: map[string]string{"app": "dd"}}},
		&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{
			Name: "p2", Namespace: "ns", Labels: map[string]string{"app": "dd"}}},
		&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{
			Name: "p3", Namespace: "ns", Labels: map[string]string{"app": "other"}}},
	}
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(pvcs...).Build()

	if err := AssertPVCCount(context.Background(), c, "ns", "app=dd", 2)(); err != nil {
		t.Fatalf("want ok, got %v", err)
	}
	if err := AssertPVCCount(context.Background(), c, "ns", "app=dd", 3)(); err == nil {
		t.Fatalf("want mismatch error")
	}
	// Malformed selector surfaces on every call.
	if err := AssertPVCCount(context.Background(), c, "ns", "!!bad!!", 0)(); err == nil {
		t.Fatalf("want parse error")
	}
}

func TestAssertTLSSecretReady(t *testing.T) {
	t.Parallel()
	s := newScheme(t)
	good := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "g", Namespace: "ns"},
		Type:       corev1.SecretTypeTLS,
		Data:       map[string][]byte{corev1.TLSCertKey: []byte("c"), corev1.TLSPrivateKeyKey: []byte("k")},
	}
	missingKey := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "ns"},
		Data:       map[string][]byte{corev1.TLSCertKey: []byte("c")},
	}
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(good, missingKey).Build()
	if err := AssertTLSSecretReady(context.Background(), c, "ns", "g")(); err != nil {
		t.Fatalf("good: %v", err)
	}
	if err := AssertTLSSecretReady(context.Background(), c, "ns", "b")(); err == nil {
		t.Fatalf("want error for missing key")
	}
	err := AssertTLSSecretReady(context.Background(), c, "ns", "none")()
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("want not-found error, got %v", err)
	}
}

func TestAssertServiceType(t *testing.T) {
	t.Parallel()
	s := newScheme(t)
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "ns"},
		Spec:       corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer},
	}
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(svc).Build()
	if err := AssertServiceType(context.Background(), c, "ns", "svc", corev1.ServiceTypeLoadBalancer)(); err != nil {
		t.Fatalf("want ok, got %v", err)
	}
	if err := AssertServiceType(context.Background(), c, "ns", "svc", corev1.ServiceTypeClusterIP)(); err == nil {
		t.Fatalf("want mismatch")
	}
}

func TestAssertConnectionStringMatches(t *testing.T) {
	t.Parallel()
	s := newScheme(t)
	dd := &preview.DocumentDB{
		ObjectMeta: metav1.ObjectMeta{Name: "db", Namespace: "ns"},
		Status:     preview.DocumentDBStatus{ConnectionString: "mongodb://user:pw@svc:10260/?tls=true"},
	}
	empty := &preview.DocumentDB{ObjectMeta: metav1.ObjectMeta{Name: "empty", Namespace: "ns"}}
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(dd, empty).Build()
	k := client.ObjectKey{Namespace: "ns", Name: "db"}

	if err := AssertConnectionStringMatches(context.Background(), c, k, `^mongodb://.*tls=true`)(); err != nil {
		t.Fatalf("want ok, got %v", err)
	}
	if err := AssertConnectionStringMatches(context.Background(), c, k, `tls=false`)(); err == nil {
		t.Fatalf("want mismatch")
	}
	if err := AssertConnectionStringMatches(context.Background(), c,
		client.ObjectKey{Namespace: "ns", Name: "empty"}, `.*`)(); err == nil {
		t.Fatalf("want empty-string error")
	}
	// Bad regex must surface.
	if err := AssertConnectionStringMatches(context.Background(), c, k, `[unclosed`)(); err == nil {
		t.Fatalf("want regex compile error")
	}
}

// psaRestrictedSC returns a SecurityContext satisfying every field
// checkPSARestricted requires.
func psaRestrictedSC() *corev1.SecurityContext {
	yes, no := true, false
	return &corev1.SecurityContext{
		RunAsNonRoot:             &yes,
		AllowPrivilegeEscalation: &no,
		Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
		SeccompProfile:           &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
	}
}

// clusterPod builds a CNPG instance pod: the cluster label the assertion
// selects on, plus the podRole marking it an instance rather than a Job pod.
func clusterPod(name, cluster string, ctrs ...corev1.Container) *corev1.Pod {
	p := jobPod(name, cluster, ctrs...)
	p.Labels[cnpgPodRoleLabel] = cnpgPodRoleInstance
	return p
}

// jobPod builds a CNPG bootstrap/join Job pod. CNPG stamps cnpg.io/cluster on
// these too, but the sidecar injector does not touch them.
func jobPod(name, cluster string, ctrs ...corev1.Container) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "ns",
			Labels:    map[string]string{"cnpg.io/cluster": cluster},
		},
		Spec: corev1.PodSpec{Containers: ctrs},
	}
}

func TestAssertInjectedSidecarsPSARestricted(t *testing.T) {
	t.Parallel()
	s := newScheme(t)

	compliant := clusterPod("ok-0", "ok",
		corev1.Container{Name: "postgres"},
		corev1.Container{Name: "documentdb-gateway", SecurityContext: psaRestrictedSC()},
	)
	// Gateway without a securityContext at all — the #387 regression.
	bare := clusterPod("bare-0", "bare",
		corev1.Container{Name: "documentdb-gateway"},
	)
	// Gateway that runs as root.
	rootSC := psaRestrictedSC()
	rootSC.RunAsNonRoot = nil
	asRoot := clusterPod("root-0", "root",
		corev1.Container{Name: "documentdb-gateway", SecurityContext: rootSC},
	)
	// No injected sidecar on the pod at all.
	noSidecar := clusterPod("none-0", "none",
		corev1.Container{Name: "postgres"},
	)

	c := fake.NewClientBuilder().WithScheme(s).
		WithObjects(compliant, bare, asRoot, noSidecar).Build()
	ctx := context.Background()

	if err := AssertInjectedSidecarsPSARestricted(ctx, c, "ns", "ok")(); err != nil {
		t.Fatalf("compliant: %v", err)
	}
	if err := AssertInjectedSidecarsPSARestricted(ctx, c, "ns", "bare")(); err == nil ||
		!strings.Contains(err.Error(), "no securityContext") {
		t.Fatalf("want missing-securityContext error, got %v", err)
	}
	if err := AssertInjectedSidecarsPSARestricted(ctx, c, "ns", "root")(); err == nil ||
		!strings.Contains(err.Error(), "runAsNonRoot") {
		t.Fatalf("want runAsNonRoot error, got %v", err)
	}
	if err := AssertInjectedSidecarsPSARestricted(ctx, c, "ns", "none")(); err == nil ||
		!strings.Contains(err.Error(), "no injected sidecar") {
		t.Fatalf("want no-injected-sidecar error, got %v", err)
	}
	if err := AssertInjectedSidecarsPSARestricted(ctx, c, "ns", "absent")(); err == nil ||
		!strings.Contains(err.Error(), "no pods found") {
		t.Fatalf("want no-pods error, got %v", err)
	}
}

func TestAssertInjectedSidecarsPSARestrictedRequiredSidecars(t *testing.T) {
	t.Parallel()
	s := newScheme(t)

	// A monitoring-on cluster whose otel-collector never got injected. The
	// always-present gateway is compliant, so the pod looks healthy.
	noOtel := clusterPod("mon-0", "mon",
		corev1.Container{Name: "postgres"},
		corev1.Container{Name: "documentdb-gateway", SecurityContext: psaRestrictedSC()},
	)
	withOtel := clusterPod("full-0", "full",
		corev1.Container{Name: "postgres"},
		corev1.Container{Name: "documentdb-gateway", SecurityContext: psaRestrictedSC()},
		corev1.Container{Name: "otel-collector", SecurityContext: psaRestrictedSC()},
	)
	otelRoot := psaRestrictedSC()
	otelRoot.SeccompProfile = nil
	badOtel := clusterPod("badotel-0", "badotel",
		corev1.Container{Name: "documentdb-gateway", SecurityContext: psaRestrictedSC()},
		corev1.Container{Name: "otel-collector", SecurityContext: otelRoot},
	)

	c := fake.NewClientBuilder().WithScheme(s).
		WithObjects(noOtel, withOtel, badOtel).Build()
	ctx := context.Background()

	// Without an explicit requirement the checker cannot tell "monitoring is
	// off" from "monitoring is on but otel never got injected": the gateway
	// satisfies it either way. That is correct for monitoring-off callers.
	if err := AssertInjectedSidecarsPSARestricted(ctx, c, "ns", "mon")(); err != nil {
		t.Fatalf("gateway-only cluster: %v", err)
	}
	// Naming otel-collector as required turns the missing sidecar into a failure.
	if err := AssertInjectedSidecarsPSARestricted(ctx, c, "ns", "mon", "otel-collector")(); err == nil ||
		!strings.Contains(err.Error(), "otel-collector") {
		t.Fatalf("want missing-otel error, got %v", err)
	}
	if err := AssertInjectedSidecarsPSARestricted(ctx, c, "ns", "full", "otel-collector")(); err != nil {
		t.Fatalf("otel present and compliant: %v", err)
	}
	if err := AssertInjectedSidecarsPSARestricted(ctx, c, "ns", "badotel", "otel-collector")(); err == nil ||
		!strings.Contains(err.Error(), "seccompProfile") {
		t.Fatalf("want otel seccomp error, got %v", err)
	}
	// A name that is not an injected sidecar is a spec bug; fail fast rather
	// than spin in Eventually until the timeout.
	if err := AssertInjectedSidecarsPSARestricted(ctx, c, "ns", "full", "postgres")(); err == nil ||
		!strings.Contains(err.Error(), "not a CNPG-I-injected sidecar") {
		t.Fatalf("want unknown-sidecar error, got %v", err)
	}
}

func TestAssertInjectedSidecarsPSARestrictedIgnoresJobPods(t *testing.T) {
	t.Parallel()
	s := newScheme(t)

	// CNPG labels its bootstrap/join Job pods with cnpg.io/cluster, but the
	// sidecar injector never adds containers to them. A lingering Job pod must
	// not read as an instance that lost its collector.
	instance := clusterPod("jobs-1", "jobs",
		corev1.Container{Name: "documentdb-gateway", SecurityContext: psaRestrictedSC()},
		corev1.Container{Name: "otel-collector", SecurityContext: psaRestrictedSC()},
	)
	bootstrap := jobPod("jobs-1-initdb", "jobs",
		corev1.Container{Name: "bootstrap-controller"},
	)
	// A cluster whose only pod is a bootstrap Job: nothing to check yet, and
	// requiring a sidecar must not pass vacuously.
	onlyJob := jobPod("early-1-initdb", "early",
		corev1.Container{Name: "bootstrap-controller"},
	)

	c := fake.NewClientBuilder().WithScheme(s).
		WithObjects(instance, bootstrap, onlyJob).Build()
	ctx := context.Background()

	if err := AssertInjectedSidecarsPSARestricted(ctx, c, "ns", "jobs", "otel-collector")(); err != nil {
		t.Fatalf("job pod alongside a healthy instance: %v", err)
	}
	if err := AssertInjectedSidecarsPSARestricted(ctx, c, "ns", "early", "otel-collector")(); err == nil ||
		!strings.Contains(err.Error(), "no injected sidecar") {
		t.Fatalf("want no-injected-sidecar error, got %v", err)
	}
}

func TestAssertInjectedSidecarsPSARestrictedNeedsAnInstancePod(t *testing.T) {
	t.Parallel()
	s := newScheme(t)

	// Sidecars present, but on a pod that is not labelled as an instance —
	// the shape we would see if CNPG stopped stamping cnpg.io/podRole. The
	// per-pod requirement would then match nothing, so without this guard
	// requireSidecars would silently stop being enforced.
	orphan := jobPod("orphan-0", "orphan",
		corev1.Container{Name: "documentdb-gateway", SecurityContext: psaRestrictedSC()},
	)
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(orphan).Build()

	err := AssertInjectedSidecarsPSARestricted(context.Background(), c, "ns", "orphan", "otel-collector")()
	if err == nil || !strings.Contains(err.Error(), "no instance pods") {
		t.Fatalf("want no-instance-pods error, got %v", err)
	}
	// With nothing required, the same cluster is still a normal pass.
	if err := AssertInjectedSidecarsPSARestricted(context.Background(), c, "ns", "orphan")(); err != nil {
		t.Fatalf("no requirement: %v", err)
	}
}
