package zberr

import (
	"fmt"
	"strings"
)

const (
	LayerCLI                Layer = "cli"
	LayerDaemonIPC          Layer = "daemon_ipc"
	LayerControlPlane       Layer = "control_plane"
	LayerControlPlaneStore  Layer = "control_plane_store"
	LayerControlPlaneClient Layer = "control_plane_client"
	LayerStackOrchestrator  Layer = "stack_orchestrator"
	LayerRouteRuntime       Layer = "route_runtime"
	LayerComposeBackend     Layer = "compose_backend"
	LayerProcessLauncher    Layer = "process_launcher"
	LayerReadinessProbe     Layer = "readiness_probe"
	LayerSocketBridge       Layer = "socket_bridge"
	LayerAuxPublication     Layer = "auxiliary_publication"
	LayerFabricCore         Layer = "fabric_core"
)

const (
	CodeInvalidInvocation           Code = "InvalidInvocation"
	CodeStartFailed                 Code = "StartFailed"
	CodeUpFailed                    Code = "UpFailed"
	CodeBootFailed                  Code = "BootFailed"
	CodeHostClaimConflict           Code = "HostClaimConflict"
	CodeListenerClaimConflict       Code = "ListenerClaimConflict"
	CodeRouteNamespaceClaimConflict Code = "RouteNamespaceClaimConflict"
	CodeLeaseRejected               Code = "LeaseRejected"
	CodePublicationRejected         Code = "PublicationRejected"
	CodePublicationDegraded         Code = "PublicationDegraded"
	CodeStaleTokenRejected          Code = "StaleTokenRejected"
	CodeRecoveryConflict            Code = "RecoveryConflict"
	CodeRouteRuntimeUnavailable     Code = "RouteRuntimeUnavailable"
	CodeStoreUnavailable            Code = "StoreUnavailable"
	CodeCorruptState                Code = "CorruptState"
	CodeCritical                    Code = "Critical"
	CodeSocketBindFailed            Code = "SocketBindFailed"
	CodeHealthCheckFailed           Code = "HealthCheckFailed"
	CodeProtocolDecodeFailed        Code = "ProtocolDecodeFailed"
	CodeDispatchFailed              Code = "DispatchFailed"
	CodeOpenFailed                  Code = "OpenFailed"
	CodeLockHeld                    Code = "LockHeld"
	CodeReadFailed                  Code = "ReadFailed"
	CodeWriteFailed                 Code = "WriteFailed"
	CodeAtomicCommitFailed          Code = "AtomicCommitFailed"
	CodeConnectionFailed            Code = "ConnectionFailed"
	CodeConfigInvalid               Code = "ConfigInvalid"
	CodeClaimRejected               Code = "ClaimRejected"
	CodePreflightRejected           Code = "PreflightRejected"
	CodeDependencyStartFailed       Code = "DependencyStartFailed"
	CodeReadinessFailed             Code = "ReadinessFailed"
	CodeGenerationCommitFailed      Code = "GenerationCommitFailed"
	CodeCleanupFailed               Code = "CleanupFailed"
	CodeBootstrapFailed             Code = "BootstrapFailed"
	CodeReloadFailed                Code = "ReloadFailed"
	CodeApplyRejected               Code = "ApplyRejected"
	CodeUnsupportedTransport        Code = "UnsupportedTransport"
	CodeModelInvalid                Code = "ModelInvalid"
	CodeIsolationRejected           Code = "IsolationRejected"
	CodeLifecycleFailed             Code = "LifecycleFailed"
	CodeSpawnFailed                 Code = "SpawnFailed"
	CodeExitedEarly                 Code = "ExitedEarly"
	CodeShutdownFailed              Code = "ShutdownFailed"
	CodeProbeInvalid                Code = "ProbeInvalid"
	CodeTimeout                     Code = "Timeout"
	CodeProbeFailed                 Code = "ProbeFailed"
	CodeBridgeInvalid               Code = "BridgeInvalid"
	CodeBridgeStartFailed           Code = "BridgeStartFailed"
	CodeBridgeHealthFailed          Code = "BridgeHealthFailed"
	CodeBridgeStopFailed            Code = "BridgeStopFailed"
	CodeSharedDomainPublishFailed   Code = "SharedDomainPublishFailed"
	CodeMetadataRenderFailed        Code = "MetadataRenderFailed"
	CodeMetadataCacheFailed         Code = "MetadataCacheFailed"
	CodeIdentityInvalid             Code = "IdentityInvalid"
	CodeNamespaceInvalid            Code = "NamespaceInvalid"
	CodeEndpointPlanInvalid         Code = "EndpointPlanInvalid"
	CodeLifecycleInvalid            Code = "LifecycleInvalid"
	CodeTemplateRenderInvalid       Code = "TemplateRenderInvalid"
)

type Declaration struct {
	Layer Layer
	Code  Code
	Kind  Kind
}

var layers = []Layer{
	LayerCLI,
	LayerDaemonIPC,
	LayerControlPlane,
	LayerControlPlaneStore,
	LayerControlPlaneClient,
	LayerStackOrchestrator,
	LayerRouteRuntime,
	LayerComposeBackend,
	LayerProcessLauncher,
	LayerReadinessProbe,
	LayerSocketBridge,
	LayerAuxPublication,
	LayerFabricCore,
}

var declarations = []Declaration{
	errDecl(LayerCLI, CodeInvalidInvocation),
	errDecl(LayerCLI, CodeStartFailed),
	errDecl(LayerCLI, CodeUpFailed),
	errDecl(LayerCLI, CodeCritical),

	errDecl(LayerDaemonIPC, CodeSocketBindFailed),
	errDecl(LayerDaemonIPC, CodeHealthCheckFailed),
	errDecl(LayerDaemonIPC, CodeProtocolDecodeFailed),
	errDecl(LayerDaemonIPC, CodeDispatchFailed),
	errDecl(LayerDaemonIPC, CodeCritical),

	errDecl(LayerControlPlane, CodeBootFailed),
	errDecl(LayerControlPlane, CodeHostClaimConflict),
	errDecl(LayerControlPlane, CodeListenerClaimConflict),
	errDecl(LayerControlPlane, CodeRouteNamespaceClaimConflict),
	errDecl(LayerControlPlane, CodeLeaseRejected),
	errDecl(LayerControlPlane, CodePublicationRejected),
	statusDecl(LayerControlPlane, CodePublicationDegraded),
	errDecl(LayerControlPlane, CodeStaleTokenRejected),
	errDecl(LayerControlPlane, CodeRecoveryConflict),
	errDecl(LayerControlPlane, CodeRouteRuntimeUnavailable),
	errDecl(LayerControlPlane, CodeStoreUnavailable),
	errDecl(LayerControlPlane, CodeCorruptState),
	errDecl(LayerControlPlane, CodeCritical),

	errDecl(LayerControlPlaneStore, CodeOpenFailed),
	errDecl(LayerControlPlaneStore, CodeLockHeld),
	errDecl(LayerControlPlaneStore, CodeReadFailed),
	errDecl(LayerControlPlaneStore, CodeWriteFailed),
	errDecl(LayerControlPlaneStore, CodeCorruptState),
	errDecl(LayerControlPlaneStore, CodeAtomicCommitFailed),
	errDecl(LayerControlPlaneStore, CodeCritical),

	errDecl(LayerControlPlaneClient, CodeConnectionFailed),
	errDecl(LayerControlPlaneClient, CodeCritical),

	errDecl(LayerStackOrchestrator, CodeConfigInvalid),
	errDecl(LayerStackOrchestrator, CodeClaimRejected),
	errDecl(LayerStackOrchestrator, CodePreflightRejected),
	errDecl(LayerStackOrchestrator, CodeDependencyStartFailed),
	errDecl(LayerStackOrchestrator, CodeReadinessFailed),
	errDecl(LayerStackOrchestrator, CodeGenerationCommitFailed),
	errDecl(LayerStackOrchestrator, CodeCleanupFailed),
	errDecl(LayerStackOrchestrator, CodeCritical),

	errDecl(LayerRouteRuntime, CodeBootstrapFailed),
	errDecl(LayerRouteRuntime, CodeReloadFailed),
	errDecl(LayerRouteRuntime, CodeApplyRejected),
	errDecl(LayerRouteRuntime, CodeUnsupportedTransport),
	errDecl(LayerRouteRuntime, CodeCritical),

	errDecl(LayerComposeBackend, CodeModelInvalid),
	errDecl(LayerComposeBackend, CodeIsolationRejected),
	errDecl(LayerComposeBackend, CodeLifecycleFailed),
	errDecl(LayerComposeBackend, CodeCritical),

	errDecl(LayerProcessLauncher, CodeSpawnFailed),
	errDecl(LayerProcessLauncher, CodeExitedEarly),
	errDecl(LayerProcessLauncher, CodeShutdownFailed),
	errDecl(LayerProcessLauncher, CodeCritical),

	errDecl(LayerReadinessProbe, CodeProbeInvalid),
	errDecl(LayerReadinessProbe, CodeTimeout),
	errDecl(LayerReadinessProbe, CodeProbeFailed),
	errDecl(LayerReadinessProbe, CodeCritical),

	errDecl(LayerSocketBridge, CodeBridgeInvalid),
	errDecl(LayerSocketBridge, CodeBridgeStartFailed),
	errDecl(LayerSocketBridge, CodeBridgeHealthFailed),
	errDecl(LayerSocketBridge, CodeBridgeStopFailed),
	errDecl(LayerSocketBridge, CodeCritical),

	errDecl(LayerAuxPublication, CodeSharedDomainPublishFailed),
	errDecl(LayerAuxPublication, CodeMetadataRenderFailed),
	errDecl(LayerAuxPublication, CodeMetadataCacheFailed),
	errDecl(LayerAuxPublication, CodeCritical),

	errDecl(LayerFabricCore, CodeConfigInvalid),
	errDecl(LayerFabricCore, CodeIdentityInvalid),
	errDecl(LayerFabricCore, CodeNamespaceInvalid),
	errDecl(LayerFabricCore, CodeEndpointPlanInvalid),
	errDecl(LayerFabricCore, CodeLifecycleInvalid),
	errDecl(LayerFabricCore, CodeTemplateRenderInvalid),
	errDecl(LayerFabricCore, CodeCritical),
}

func errDecl(layer Layer, code Code) Declaration {
	return Declaration{Layer: layer, Code: code, Kind: KindError}
}

func statusDecl(layer Layer, code Code) Declaration {
	return Declaration{Layer: layer, Code: code, Kind: KindStatus}
}

func Layers() []Layer {
	return append([]Layer(nil), layers...)
}

func Declarations() []Declaration {
	return append([]Declaration(nil), declarations...)
}

func Lookup(layer Layer, code Code) (Declaration, bool) {
	for _, declaration := range declarations {
		if declaration.Layer == layer && declaration.Code == code {
			return declaration, true
		}
	}
	return Declaration{}, false
}

func ValidateDeclarations() error {
	seen := make(map[string]Declaration, len(declarations))
	criticalByLayer := make(map[Layer]bool, len(layers))

	for _, declaration := range declarations {
		key := declarationKey(declaration.Layer, declaration.Code)
		if prior, ok := seen[key]; ok {
			return fmt.Errorf("duplicate declaration %s/%s also declared as %s", declaration.Layer, declaration.Code, prior.Kind)
		}
		seen[key] = declaration

		if declaration.Code == CodeCritical && declaration.Kind == KindError {
			criticalByLayer[declaration.Layer] = true
		}
		if declaration.Kind != KindError && declaration.Kind != KindStatus {
			return fmt.Errorf("invalid declaration kind %q for %s/%s", declaration.Kind, declaration.Layer, declaration.Code)
		}
	}

	var missing []string
	for _, layer := range layers {
		if !criticalByLayer[layer] {
			missing = append(missing, string(layer))
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing Critical declaration for layers: %s", strings.Join(missing, ", "))
	}

	return nil
}

func declarationKey(layer Layer, code Code) string {
	return string(layer) + "\x00" + string(code)
}
