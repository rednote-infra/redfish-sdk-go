// Package redfish preserves the original SDK import path.
// New integrations can import the client, errors, response, and discovery packages directly.
package redfish

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	sdkclient "github.com/rednote-infra/redfish-sdk-go/client"
	"github.com/rednote-infra/redfish-sdk-go/discovery"
	sdkerrors "github.com/rednote-infra/redfish-sdk-go/errors"
	"github.com/rednote-infra/redfish-sdk-go/internal/redaction"
	"github.com/rednote-infra/redfish-sdk-go/internal/wire"
	"github.com/rednote-infra/redfish-sdk-go/models"
	sdkresponse "github.com/rednote-infra/redfish-sdk-go/response"
)

type (
	PolicyCreate = sdkclient.PolicyCreate
	PolicyUpdate = sdkclient.PolicyUpdate
	Action       = sdkclient.Action
	ChangeStatus = sdkclient.ChangeStatus
)

const (
	ChangePending   = sdkclient.ChangePending
	ChangeValidated = sdkclient.ChangeValidated
	ChangeSucceeded = sdkclient.ChangeSucceeded
	ChangeAccepted  = sdkclient.ChangeAccepted
	ChangeFailed    = sdkclient.ChangeFailed
	ChangeBlocked   = sdkclient.ChangeBlocked
	ChangeSkipped   = sdkclient.ChangeSkipped
	ChangeUncertain = sdkclient.ChangeUncertain
)

type (
	ChangeOptions         = sdkclient.ChangeOptions
	ChangeValidator       = sdkclient.ChangeValidator
	ChangeOperation       = sdkclient.ChangeOperation
	ChangeValidationIssue = sdkclient.ChangeValidationIssue
	ChangeOperationResult = sdkclient.ChangeOperationResult
	ChangeSetResult       = sdkclient.ChangeSetResult
	CommitOptions         = sdkclient.CommitOptions
	ChangeSet             = sdkclient.ChangeSet
)

const (
	DefaultServiceRoot = wire.DefaultServiceRoot
	HeaderAuthToken    = wire.HeaderAuthToken
	HeaderIfMatch      = wire.HeaderIfMatch
	HeaderETag         = wire.HeaderETag
	HeaderLocation     = wire.HeaderLocation
	HeaderRetryAfter   = wire.HeaderRetryAfter
	HeaderODataVersion = wire.HeaderODataVersion
	ODataVersion       = wire.ODataVersion
)

type AuthenticationMode = sdkclient.AuthenticationMode

const (
	AuthenticationBasic   = sdkclient.AuthenticationBasic
	AuthenticationSession = sdkclient.AuthenticationSession
)

type Object = sdkclient.Object

func Ptr[T any](value T) *T { return models.Ptr[T](value) }

func Value[T any](value *T) (zero T) { return models.Value[T](value) }

type (
	RetryPolicy = sdkclient.RetryPolicy
	RetrySafety = sdkclient.RetrySafety
	Option      = sdkclient.Option
)

func WithAuthentication(mode AuthenticationMode) Option { return sdkclient.WithAuthentication(mode) }

func WithHTTPClient(client *http.Client) Option { return sdkclient.WithHTTPClient(client) }

func WithExternalHTTPClient(client *http.Client) Option {
	return sdkclient.WithExternalHTTPClient(client)
}

const (
	RetrySafetyDefault    = sdkclient.RetrySafetyDefault
	RetrySafetyIdempotent = sdkclient.RetrySafetyIdempotent
)

func WithTLSVerification(verify bool) Option { return sdkclient.WithTLSVerification(verify) }

func WithCABundle(path string) Option { return sdkclient.WithCABundle(path) }

func WithProxy(proxy string) Option { return sdkclient.WithProxy(proxy) }

func WithTimeout(timeout time.Duration) Option { return sdkclient.WithTimeout(timeout) }

func WithConnectTimeout(timeout time.Duration) Option { return sdkclient.WithConnectTimeout(timeout) }

func WithServiceRoot(path string) Option { return sdkclient.WithServiceRoot(path) }

func WithRetry(policy RetryPolicy) Option { return sdkclient.WithRetry(policy) }

func WithSession(token, location string) Option { return sdkclient.WithSession(token, location) }

func WithVendor(vendor Vendor) Option { return sdkclient.WithVendor(vendor) }

func WithProviders(registry *ProviderRegistry) Option { return sdkclient.WithProviders(registry) }

func WithStrictCollections(strict bool) Option { return sdkclient.WithStrictCollections(strict) }

type Client = sdkclient.Client

func NewClient(host, username, password string, options ...Option) (*Client, error) {
	return sdkclient.NewClient(host, username, password, options...)
}

type (
	RequestOptions          = sdkclient.RequestOptions
	Response                = sdkresponse.Response
	CollectionFailure       = sdkclient.CollectionFailure
	CollectionResult[T any] = sdkclient.CollectionResult[T]
)

func Collect[T any](ctx context.Context, c *Client, uri string) (CollectionResult[T], error) {
	return sdkclient.Collect[T](ctx, c, uri)
}

func GetResourceODataID(resource any) (string, error) { return sdkclient.GetResourceODataID(resource) }

type PartialCollectionError = sdkclient.PartialCollectionError

const (
	ConsoleVNC      = sdkclient.ConsoleVNC
	ConsoleHTML5    = sdkclient.ConsoleHTML5
	ConsoleRDP      = sdkclient.ConsoleRDP
	ConsoleKVMIP    = sdkclient.ConsoleKVMIP
	AccessReadWrite = sdkclient.AccessReadWrite
	AccessReadOnly  = sdkclient.AccessReadOnly
)

type (
	ConsoleOptions            = sdkclient.ConsoleOptions
	DiagnosticOptions         = sdkclient.DiagnosticOptions
	DiagnosticTask            = sdkclient.DiagnosticTask
	CollectDownloadOptions    = sdkclient.CollectDownloadOptions
	DiagnosticLifecycle       = sdkclient.DiagnosticLifecycle
	DiagnosticBytesDownloader = sdkclient.DiagnosticBytesDownloader
	DiscoveryProtocol         = discovery.DiscoveryProtocol
)

const (
	DiscoveryIPv4 = discovery.DiscoveryIPv4
	DiscoveryIPv6 = discovery.DiscoveryIPv6
)

type (
	SSDPOptions       = discovery.SSDPOptions
	DiscoveredService = discovery.DiscoveredService
)

func ParseSSDPResponse(data []byte, source string) (*DiscoveredService, error) {
	return discovery.ParseSSDPResponse(data, source)
}

func DiscoverSSDP(ctx context.Context, options SSDPOptions) ([]DiscoveredService, error) {
	return discovery.DiscoverSSDP(ctx, options)
}

var (
	ErrClosed                 = sdkerrors.ErrClosed
	ErrNotFound               = sdkerrors.ErrNotFound
	ErrUnauthorized           = sdkerrors.ErrUnauthorized
	ErrForbidden              = sdkerrors.ErrForbidden
	ErrConflict               = sdkerrors.ErrConflict
	ErrPreconditionFailed     = sdkerrors.ErrPreconditionFailed
	ErrRateLimit              = sdkerrors.ErrRateLimit
	ErrPasswordChangeRequired = sdkerrors.ErrPasswordChangeRequired
)

type Error = sdkerrors.Error

func IsStatus(err error, status int) bool { return sdkerrors.IsStatus(err, status) }

type TransportError = sdkerrors.TransportError

func Redact(value any, secrets ...string) (json.RawMessage, error) {
	return redaction.Redact(value, secrets...)
}

type (
	MissingAttributeError = sdkerrors.MissingAttributeError
	PortSelector          = sdkclient.PortSelector
	SerialInterfaceUpdate = sdkclient.SerialInterfaceUpdate
	Vendor                = sdkclient.Vendor
)

const (
	VendorGeneric       = sdkclient.VendorGeneric
	VendorAMI           = sdkclient.VendorAMI
	VendorZTE           = sdkclient.VendorZTE
	VendorInspur        = sdkclient.VendorInspur
	VendorIEIT          = sdkclient.VendorIEIT
	VendorH3C           = sdkclient.VendorH3C
	VendorXFusion       = sdkclient.VendorXFusion
	VendorLenovo        = sdkclient.VendorLenovo
	VendorNettrix       = sdkclient.VendorNettrix
	VendorEnginetech    = sdkclient.VendorEnginetech
	VendorSmoothcompute = sdkclient.VendorSmoothcompute
)

type (
	Extractor               = sdkclient.Extractor
	UpdateStrategy          = sdkclient.UpdateStrategy
	LogCollectStrategy      = sdkclient.LogCollectStrategy
	ProviderRegistry        = sdkclient.ProviderRegistry
	ProviderRegistryOptions = sdkclient.ProviderRegistryOptions
	ProviderRegistration    = sdkclient.ProviderRegistration
	ProviderDiscovery       = sdkclient.ProviderDiscovery
	OEMCategory             = sdkclient.OEMCategory
	UpdateExecutor          = sdkclient.UpdateExecutor
	ProviderConflictError   = sdkerrors.ProviderConflictError
	ProviderLoadError       = sdkerrors.ProviderLoadError
)

func NewProviderRegistry() *ProviderRegistry { return sdkclient.NewProviderRegistry() }

func NewProviderRegistryWithOptions(options ProviderRegistryOptions) *ProviderRegistry {
	return sdkclient.NewProviderRegistryWithOptions(options)
}

const (
	OEMExtractor  = sdkclient.OEMExtractor
	OEMUpdate     = sdkclient.OEMUpdate
	OEMLogCollect = sdkclient.OEMLogCollect
)

func DetectVendor(manufacturer string, oem *models.Oem) Vendor {
	return sdkclient.DetectVendor(manufacturer, oem)
}

func ValidateAttributeValues(registry *models.AttributeRegistry, values Object) error {
	return sdkclient.ValidateAttributeValues(registry, values)
}

const (
	DefaultDiscoveryMaxDepth     = sdkclient.DefaultDiscoveryMaxDepth
	DefaultDiscoveryMaxResources = sdkclient.DefaultDiscoveryMaxResources
)

type (
	DiscoveryOptions         = sdkclient.DiscoveryOptions
	ResourceDescriptor       = sdkclient.ResourceDescriptor
	ResourceDiscoveryFailure = sdkclient.ResourceDiscoveryFailure
	ResourceIndex            = sdkclient.ResourceIndex
	ApplyTime                = sdkclient.ApplyTime
)

const (
	ApplyImmediate                  = sdkclient.ApplyImmediate
	ApplyOnReset                    = sdkclient.ApplyOnReset
	ApplyAtMaintenanceWindowStart   = sdkclient.ApplyAtMaintenanceWindowStart
	ApplyInMaintenanceWindowOnReset = sdkclient.ApplyInMaintenanceWindowOnReset
	BootModeUEFI                    = sdkclient.BootModeUEFI
	BootModeLegacy                  = sdkclient.BootModeLegacy
	BootOverrideOnce                = sdkclient.BootOverrideOnce
	BootOverrideContinuous          = sdkclient.BootOverrideContinuous
	BootOverrideDisabled            = sdkclient.BootOverrideDisabled
	ResetOn                         = sdkclient.ResetOn
	ResetForceOff                   = sdkclient.ResetForceOff
	ResetGracefulShutdown           = sdkclient.ResetGracefulShutdown
	ResetGracefulRestart            = sdkclient.ResetGracefulRestart
	ResetForceRestart               = sdkclient.ResetForceRestart
	ResetPowerCycle                 = sdkclient.ResetPowerCycle
)

type (
	BIOSOptions  = sdkclient.BIOSOptions
	BootOptions  = sdkclient.BootOptions
	ResetOptions = sdkclient.ResetOptions
)

const (
	TaskNew         = sdkclient.TaskNew
	TaskRunning     = sdkclient.TaskRunning
	TaskPending     = sdkclient.TaskPending
	TaskCompleted   = sdkclient.TaskCompleted
	TaskException   = sdkclient.TaskException
	TaskKilled      = sdkclient.TaskKilled
	TaskCancelled   = sdkclient.TaskCancelled
	TaskInterrupted = sdkclient.TaskInterrupted
)

type (
	PollOptions         = sdkclient.PollOptions
	AsyncOperationError = sdkclient.AsyncOperationError
	OperationResult     = sdkclient.OperationResult
	UpdateRequest       = sdkclient.UpdateRequest
)

const (
	FlashFull                 = sdkclient.FlashFull
	FlashDualImage            = sdkclient.FlashDualImage
	MultipartUpdateFile       = sdkclient.MultipartUpdateFile
	MultipartUpdateParameters = sdkclient.MultipartUpdateParameters
	MultipartOEMParameters    = sdkclient.MultipartOEMParameters
)

type UploadOptions = sdkclient.UploadOptions

const (
	FieldImageURI                            = wire.FieldImageURI
	FieldTransferProtocol                    = wire.FieldTransferProtocol
	FieldTargets                             = wire.FieldTargets
	FieldExcludeTargets                      = wire.FieldExcludeTargets
	FieldUsername                            = wire.FieldUsername
	FieldUserName                            = wire.FieldUserName
	FieldPassword                            = wire.FieldPassword
	FieldOEM                                 = wire.FieldOEM
	FieldPublic                              = wire.FieldPublic
	FieldType                                = wire.FieldType
	FieldContent                             = wire.FieldContent
	FieldPath                                = wire.FieldPath
	FieldDiagnosticDataType                  = wire.FieldDiagnosticDataType
	FieldOEMDiagnosticDataType               = wire.FieldOEMDiagnosticDataType
	FieldResetType                           = wire.FieldResetType
	FieldResetKeysType                       = wire.FieldResetKeysType
	FieldEnabled                             = wire.FieldEnabled
	FieldName                                = wire.FieldName
	FieldID                                  = wire.FieldID
	FieldAttributes                          = wire.FieldAttributes
	FieldBoot                                = wire.FieldBoot
	FieldConsoleType                         = wire.FieldConsoleType
	FieldAccessMode                          = wire.FieldAccessMode
	FieldValidityPeriodSeconds               = wire.FieldValidityPeriodSeconds
	FieldPowerState                          = wire.FieldPowerState
	FieldStatus                              = wire.FieldStatus
	FieldTaskState                           = wire.FieldTaskState
	FieldPreserveConf                        = wire.FieldPreserveConf
	FieldPreserve                            = wire.FieldPreserve
	FieldFlashItem                           = wire.FieldFlashItem
	FieldBIOSUpdateType                      = wire.FieldBIOSUpdateType
	FieldBiosFlash                           = wire.FieldBiosFlash
	FieldBMCFlash                            = wire.FieldBMCFlash
	FieldApplyTime                           = wire.FieldApplyTime
	FieldStage                               = wire.FieldStage
	FieldForceUpdate                         = wire.FieldForceUpdate
	FieldLocalImage                          = wire.FieldLocalImage
	ActionSimpleUpdate                       = wire.ActionSimpleUpdate
	ActionCollectDiagnosticData              = wire.ActionCollectDiagnosticData
	ActionCollectAllLog                      = wire.ActionCollectAllLog
	ActionDump                               = wire.ActionDump
	ActionDownloadAllLog                     = wire.ActionDownloadAllLog
	ActionExportLogs                         = wire.ActionExportLogs
	ActionGeneralDownload                    = wire.ActionGeneralDownload
	ActionGetLogCollectProgress              = wire.ActionGetLogCollectProgress
	ActionClearLog                           = wire.ActionClearLog
	ActionActivate                           = wire.ActionActivate
	ActionUploadSignKey                      = wire.ActionUploadSignKey
	ActionUploadFirmwareImage                = wire.ActionUploadFirmwareImage
	ActionBMCFwUpdate                        = wire.ActionBMCFwUpdate
	ActionInsertMedia                        = wire.ActionInsertMedia
	ActionEjectMedia                         = wire.ActionEjectMedia
	ActionGraphicalConsoleGetOneTimeURL      = wire.ActionGraphicalConsoleGetOneTimeURL
	ActionGraphicalConsoleSetOneTimePassword = wire.ActionGraphicalConsoleSetOneTimePassword
	ActionGraphicalConsoleSetPassword        = wire.ActionGraphicalConsoleSetPassword
	TransferProtocolHTTP                     = wire.TransferProtocolHTTP
	TransferProtocolHTTPS                    = wire.TransferProtocolHTTPS
	DiagnosticOutputURI                      = wire.DiagnosticOutputURI
	OEMNamespaceXFusion                      = wire.OEMNamespaceXFusion
	OEMNamespaceAMI                          = wire.OEMNamespaceAMI
	OEMNamespaceCustom                       = wire.OEMNamespaceCustom
	OEMNamespaceLenovo                       = wire.OEMNamespaceLenovo
	PowerStateOn                             = wire.PowerStateOn
	PowerStateOff                            = wire.PowerStateOff
	PowerStatePoweringOn                     = wire.PowerStatePoweringOn
	PowerStatePoweringOff                    = wire.PowerStatePoweringOff
	HeaderIfNoneMatch                        = wire.HeaderIfNoneMatch
)

const HeaderContentLocation = wire.HeaderContentLocation

// WithCollectionConcurrency bounds simultaneous collection member reads.
func WithCollectionConcurrency(workers int) Option {
	return sdkclient.WithCollectionConcurrency(workers)
}

type BootRoute = sdkclient.BootRoute
type BootUpdateError = sdkclient.BootUpdateError

const (
	BootRouteCurrent  = sdkclient.BootRouteCurrent
	BootRouteAuto     = sdkclient.BootRouteAuto
	BootRouteSettings = sdkclient.BootRouteSettings
)

// CachedResource retains typed data and conditional GET response metadata.
type CachedResource[T any] = sdkclient.CachedResource[T]

func Fetch[T any](ctx context.Context, c *Client, uri string, previous *CachedResource[T]) (*CachedResource[T], error) {
	return sdkclient.Fetch(ctx, c, uri, previous)
}
