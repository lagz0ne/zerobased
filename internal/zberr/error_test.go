package zberr

import (
	"errors"
	"testing"
)

func TestErrorCarriesLayerCodePhaseOwnerCauseAndOrigin(t *testing.T) {
	cause := errors.New("disk is gone")

	err := New(LayerControlPlane, CodeStoreUnavailable,
		WithPhase("store"),
		WithOwner("stack-1"),
		WithCause(cause),
		WithOrigin(LayerControlPlaneStore),
	)

	if err.Layer != LayerControlPlane {
		t.Fatalf("Layer = %q, want %q", err.Layer, LayerControlPlane)
	}
	if err.Code != CodeStoreUnavailable {
		t.Fatalf("Code = %q, want %q", err.Code, CodeStoreUnavailable)
	}
	if err.Phase != "store" {
		t.Fatalf("Phase = %q, want store", err.Phase)
	}
	if err.Owner != "stack-1" {
		t.Fatalf("Owner = %q, want stack-1", err.Owner)
	}
	if !errors.Is(err, cause) {
		t.Fatalf("errors.Is did not unwrap cause")
	}
	if len(err.Origin) != 1 || err.Origin[0] != LayerControlPlaneStore {
		t.Fatalf("Origin = %#v, want [%q]", err.Origin, LayerControlPlaneStore)
	}
	if !Is(err, LayerControlPlane, CodeStoreUnavailable) {
		t.Fatalf("Is did not match layer/code")
	}
}

func TestCriticalDefaultsOriginToOwningLayer(t *testing.T) {
	err := Critical(LayerDaemonIPC, errors.New("panic"))

	if err.Code != CodeCritical {
		t.Fatalf("Code = %q, want %q", err.Code, CodeCritical)
	}
	if len(err.Origin) != 1 || err.Origin[0] != LayerDaemonIPC {
		t.Fatalf("Origin = %#v, want [%q]", err.Origin, LayerDaemonIPC)
	}
}

func TestCriticalPreservesLowerOriginChain(t *testing.T) {
	cause := Critical(LayerDaemonIPC, errors.New("panic"))
	err := Critical(LayerControlPlaneClient, cause)

	expected := []Layer{LayerControlPlaneClient, LayerDaemonIPC}
	if len(err.Origin) != len(expected) {
		t.Fatalf("Origin = %#v, want %#v", err.Origin, expected)
	}
	for i := range expected {
		if err.Origin[i] != expected[i] {
			t.Fatalf("Origin = %#v, want %#v", err.Origin, expected)
		}
	}
}

func TestIsMatchesWrappedZerobasedCause(t *testing.T) {
	cause := New(LayerControlPlaneStore, CodeLockHeld)
	err := New(LayerControlPlane, CodeStoreUnavailable, WithCause(cause))

	if !Is(err, LayerControlPlaneStore, CodeLockHeld) {
		t.Fatalf("Is did not match wrapped zerobased cause")
	}
}

func TestValidateRequiresPhaseForCLIStartAndUpFailures(t *testing.T) {
	for _, code := range []Code{CodeStartFailed, CodeUpFailed} {
		err := New(LayerCLI, code)
		if Validate(err) == nil {
			t.Fatalf("Validate(%s/%s) succeeded without phase", err.Layer, err.Code)
		}
	}

	if err := Validate(New(LayerCLI, CodeStartFailed, WithPhase("boot"))); err != nil {
		t.Fatalf("Validate phased StartFailed: %v", err)
	}
	if err := Validate(New(LayerCLI, CodeUpFailed, WithPhase("claim"))); err != nil {
		t.Fatalf("Validate phased UpFailed: %v", err)
	}
}

func TestDeclarationsAreUniqueAndEveryLayerHasCritical(t *testing.T) {
	if err := ValidateDeclarations(); err != nil {
		t.Fatal(err)
	}
}

func TestRegistryMatchesTracedTDDDeclaredPairs(t *testing.T) {
	expected := []Declaration{
		{LayerCLI, CodeInvalidInvocation, KindError},
		{LayerCLI, CodeStartFailed, KindError},
		{LayerCLI, CodeUpFailed, KindError},
		{LayerCLI, CodeCritical, KindError},
		{LayerDaemonIPC, CodeSocketBindFailed, KindError},
		{LayerDaemonIPC, CodeHealthCheckFailed, KindError},
		{LayerDaemonIPC, CodeProtocolDecodeFailed, KindError},
		{LayerDaemonIPC, CodeDispatchFailed, KindError},
		{LayerDaemonIPC, CodeCritical, KindError},
		{LayerControlPlane, CodeBootFailed, KindError},
		{LayerControlPlane, CodeHostClaimConflict, KindError},
		{LayerControlPlane, CodeListenerClaimConflict, KindError},
		{LayerControlPlane, CodeRouteNamespaceClaimConflict, KindError},
		{LayerControlPlane, CodeLeaseRejected, KindError},
		{LayerControlPlane, CodePublicationRejected, KindError},
		{LayerControlPlane, CodePublicationDegraded, KindStatus},
		{LayerControlPlane, CodeStaleTokenRejected, KindError},
		{LayerControlPlane, CodeRecoveryConflict, KindError},
		{LayerControlPlane, CodeRouteRuntimeUnavailable, KindError},
		{LayerControlPlane, CodeStoreUnavailable, KindError},
		{LayerControlPlane, CodeCorruptState, KindError},
		{LayerControlPlane, CodeCritical, KindError},
		{LayerControlPlaneStore, CodeOpenFailed, KindError},
		{LayerControlPlaneStore, CodeLockHeld, KindError},
		{LayerControlPlaneStore, CodeReadFailed, KindError},
		{LayerControlPlaneStore, CodeWriteFailed, KindError},
		{LayerControlPlaneStore, CodeCorruptState, KindError},
		{LayerControlPlaneStore, CodeAtomicCommitFailed, KindError},
		{LayerControlPlaneStore, CodeCritical, KindError},
		{LayerControlPlaneClient, CodeConnectionFailed, KindError},
		{LayerControlPlaneClient, CodeCritical, KindError},
		{LayerStackOrchestrator, CodeConfigInvalid, KindError},
		{LayerStackOrchestrator, CodeClaimRejected, KindError},
		{LayerStackOrchestrator, CodePreflightRejected, KindError},
		{LayerStackOrchestrator, CodeDependencyStartFailed, KindError},
		{LayerStackOrchestrator, CodeReadinessFailed, KindError},
		{LayerStackOrchestrator, CodeGenerationCommitFailed, KindError},
		{LayerStackOrchestrator, CodeCleanupFailed, KindError},
		{LayerStackOrchestrator, CodeCritical, KindError},
		{LayerRouteRuntime, CodeBootstrapFailed, KindError},
		{LayerRouteRuntime, CodeReloadFailed, KindError},
		{LayerRouteRuntime, CodeApplyRejected, KindError},
		{LayerRouteRuntime, CodeUnsupportedTransport, KindError},
		{LayerRouteRuntime, CodeCritical, KindError},
		{LayerComposeBackend, CodeModelInvalid, KindError},
		{LayerComposeBackend, CodeIsolationRejected, KindError},
		{LayerComposeBackend, CodeLifecycleFailed, KindError},
		{LayerComposeBackend, CodeCritical, KindError},
		{LayerProcessLauncher, CodeSpawnFailed, KindError},
		{LayerProcessLauncher, CodeExitedEarly, KindError},
		{LayerProcessLauncher, CodeShutdownFailed, KindError},
		{LayerProcessLauncher, CodeCritical, KindError},
		{LayerReadinessProbe, CodeProbeInvalid, KindError},
		{LayerReadinessProbe, CodeTimeout, KindError},
		{LayerReadinessProbe, CodeProbeFailed, KindError},
		{LayerReadinessProbe, CodeCritical, KindError},
		{LayerSocketBridge, CodeBridgeInvalid, KindError},
		{LayerSocketBridge, CodeBridgeStartFailed, KindError},
		{LayerSocketBridge, CodeBridgeHealthFailed, KindError},
		{LayerSocketBridge, CodeBridgeStopFailed, KindError},
		{LayerSocketBridge, CodeCritical, KindError},
		{LayerAuxPublication, CodeSharedDomainPublishFailed, KindError},
		{LayerAuxPublication, CodeMetadataRenderFailed, KindError},
		{LayerAuxPublication, CodeMetadataCacheFailed, KindError},
		{LayerAuxPublication, CodeCritical, KindError},
		{LayerFabricCore, CodeConfigInvalid, KindError},
		{LayerFabricCore, CodeIdentityInvalid, KindError},
		{LayerFabricCore, CodeNamespaceInvalid, KindError},
		{LayerFabricCore, CodeEndpointPlanInvalid, KindError},
		{LayerFabricCore, CodeLifecycleInvalid, KindError},
		{LayerFabricCore, CodeTemplateRenderInvalid, KindError},
		{LayerFabricCore, CodeCritical, KindError},
	}

	actual := Declarations()
	if len(actual) != len(expected) {
		t.Fatalf("declaration count = %d, want %d", len(actual), len(expected))
	}

	for _, declaration := range expected {
		actualDeclaration, ok := Lookup(declaration.Layer, declaration.Code)
		if !ok {
			t.Fatalf("missing declaration %#v", declaration)
		}
		if actualDeclaration.Kind != declaration.Kind {
			t.Fatalf("%s/%s kind = %q, want %q", declaration.Layer, declaration.Code, actualDeclaration.Kind, declaration.Kind)
		}
	}
}

func TestPublicationDegradedIsStatusNotError(t *testing.T) {
	decl, ok := Lookup(LayerControlPlane, CodePublicationDegraded)
	if !ok {
		t.Fatalf("PublicationDegraded declaration missing")
	}
	if decl.Kind != KindStatus {
		t.Fatalf("Kind = %q, want %q", decl.Kind, KindStatus)
	}
}
