package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	resourceprovider "github.com/rednote-infra/redfish-sdk-go/client/provider"
	sdkerrors "github.com/rednote-infra/redfish-sdk-go/errors"
	"github.com/rednote-infra/redfish-sdk-go/internal/redaction"
	"github.com/rednote-infra/redfish-sdk-go/internal/wire"
	"github.com/rednote-infra/redfish-sdk-go/models"
	"github.com/rednote-infra/redfish-sdk-go/oem"
	sdkresponse "github.com/rednote-infra/redfish-sdk-go/response"
)

const (
	_ieitTaskVisibilityDelayCode = 17017
)

type DiagnosticOptions = resourceprovider.DiagnosticOptions

type DiagnosticTask struct {
	Task                                                          *models.Task
	Response                                                      *sdkresponse.Response
	Vendor                                                        Vendor
	ProgressURI, DownloadURI, DownloadMethod, SourceLogServiceURI string
	DownloadBody                                                  Object
	PreviousArtifact                                              string
	FreshTrigger                                                  bool
}

type CollectDownloadOptions struct {
	Diagnostic   DiagnosticOptions
	Poll         PollOptions
	DisableReuse bool
	MaxRetries   int
	RetryBackoff time.Duration
}

// DiagnosticLifecycle allows custom providers to replace the complete OEM workflow.
type DiagnosticLifecycle interface {
	Collect(context.Context, *Client, DiagnosticOptions) (*DiagnosticTask, error)
	Wait(context.Context, *Client, *DiagnosticTask, PollOptions) error
	Download(context.Context, *Client, *DiagnosticTask, string) (string, error)
}

// vendorLogCollect adapts an OEM policy to the legacy Client lifecycle hooks.
type vendorLogCollect struct {
	vendor Vendor
	LogCollectStrategy
}

func builtinLogCollect(vendor Vendor) vendorLogCollect {
	return vendorLogCollect{vendor: vendor, LogCollectStrategy: oem.NewLogCollectStrategy(vendor)}
}

func (c *Client) diagnosticAction(ctx context.Context, collectionURI, name, logID string) (*Action, string, error) {
	actions, err := c.resourceActions(ctx, collectionURI)
	if err != nil {
		return nil, "", err
	}
	if action, err := findAction(actions, name); err == nil {
		return action, collectionURI, nil
	}
	services, err := collection[models.Log](ctx, c, collectionURI)
	if err != nil {
		return nil, "", err
	}
	if logID != "" {
		service, err := selectItem(services, logID)
		if err != nil {
			return nil, "", err
		}
		services = []models.Log{*service}
	}
	var selected *Action
	var source string
	for _, service := range services {
		if service.Actions == nil {
			continue
		}
		if action, err := findAction(service.Actions.Extra, name); err == nil {
			if selected != nil {
				return nil, "", errors.New("diagnostic action is ambiguous; specify LogID")
			}
			selected = action
			source = models.Value(service.ODataID)
		}
	}
	if selected == nil {
		return nil, "", errors.New("diagnostic action is not advertised: " + name)
	}
	return selected, source, nil
}

func (c *Client) CollectDiagnosticData(ctx context.Context, options DiagnosticOptions) (*DiagnosticTask, error) {
	vendor, err := c.vendor(ctx)
	if err != nil {
		return nil, err
	}
	provider, err := c.diagnosticProvider(vendor)
	if err != nil {
		return nil, err
	}
	if lifecycle, ok := provider.(DiagnosticLifecycle); ok {
		return lifecycle.Collect(ctx, c, options)
	}
	return c.collectDiagnostic(ctx, vendor, provider, options)
}

func (p vendorLogCollect) Collect(ctx context.Context, c *Client, options DiagnosticOptions) (*DiagnosticTask, error) {
	return c.collectDiagnostic(ctx, p.vendor, p, options)
}

func (c *Client) collectDiagnostic(ctx context.Context, vendor Vendor, provider LogCollectStrategy, options DiagnosticOptions) (*DiagnosticTask, error) {
	layout := diagnosticLayout(provider)
	implementation := diagnosticImplementation(provider)
	if len(options.DownloadQuery) > 0 {
		if _, ok := implementation.(resourceprovider.DiagnosticArtifactSource); !ok {
			return nil, errors.New("diagnostic provider does not support read-only download query parameters")
		}
	}

	if layout.SystemScope {
		task, err := c.collectSystemDiagnostic(ctx, provider, options)
		if task != nil {
			task.Vendor = vendor
		}
		return task, err
	}
	manager, err := c.GetManager(ctx, options.ManagerID)
	if err != nil {
		return nil, err
	}

	if source, ok := implementation.(resourceprovider.DiagnosticArtifactSource); ok {
		if _, err := provider.BuildBody(options); err != nil {
			return nil, err
		}
		serviceURI, err := source.DiagnosticServiceURI(manager)
		if err != nil {
			return nil, err
		}
		response, err := c.Request(ctx, http.MethodGet, serviceURI, nil)
		if err != nil {
			return nil, err
		}
		artifactURI, err := source.DiagnosticArtifactURI(response.Body, options)
		if err != nil {
			return nil, err
		}
		target, err := c.resolveArtifactReference(artifactURI, response.URL)
		if err != nil {
			return nil, err
		}
		return &DiagnosticTask{Vendor: vendor, Task: &models.Task{TaskState: models.Ptr(TaskCompleted)}, Response: response, DownloadURI: target, DownloadMethod: http.MethodGet}, nil
	}
	resolver, hasService := implementation.(resourceprovider.DiagnosticServiceResolver)
	var collectionURI string
	if !hasService && !layout.UseManagerActions && layout.ManagerServiceSuffix == "" {
		collectionURI, err = requireLink(manager.LogServices, "LogServices")
		if err != nil {
			return nil, err
		}
	}

	task := &DiagnosticTask{Vendor: vendor, Task: &models.Task{TaskState: models.Ptr(TaskRunning)}, DownloadMethod: http.MethodGet, FreshTrigger: true}
	body, err := provider.BuildBody(options)
	if err != nil {
		return nil, err
	}

	var target string
	var directActions map[string]json.RawMessage
	if hasService {
		serviceURI, err := resolver.DiagnosticServiceURI(manager)
		if err != nil {
			return nil, err
		}
		directActions, err = c.resourceActions(ctx, serviceURI)
		if err != nil {
			return nil, err
		}
	} else if layout.UseManagerActions {
		if manager.Actions == nil {
			return nil, errors.New("manager does not advertise diagnostic actions")
		}
		directActions = manager.Actions.Extra
	} else if layout.ManagerServiceSuffix != "" {
		directActions, err = c.resourceActions(ctx, strings.TrimRight(models.Value(manager.ODataID), "/")+layout.ManagerServiceSuffix)
		if err != nil {
			return nil, err
		}
	}
	if hasService || layout.UseManagerActions || layout.ManagerServiceSuffix != "" {
		action, resolveErr := findAction(directActions, layout.CollectAction)
		if resolveErr != nil && layout.FallbackCollectAction != "" {
			collectionURI, err = requireLink(manager.LogServices, "LogServices")
			if err != nil {
				return nil, err
			}
			action, task.SourceLogServiceURI, resolveErr = c.diagnosticAction(ctx, collectionURI, layout.FallbackCollectAction, options.LogID)
		}
		if resolveErr != nil {
			return nil, resolveErr
		}
		target = action.Target
		if layout.DownloadAction != "" {
			download, err := findAction(directActions, layout.DownloadAction)
			if err != nil {
				return nil, err
			}
			task.DownloadURI = download.Target
			task.DownloadMethod = http.MethodPost
		}
		if builder, ok := diagnosticImplementation(provider).(resourceprovider.DiagnosticDownloadBuilder); ok {
			task.DownloadBody = builder.BuildDownloadBody(body)
		}
	}
	if target == "" && layout.SelectService {
		services, err := collection[models.Log](ctx, c, collectionURI)
		if err != nil {
			return nil, err
		}
		if options.LogID != "" {
			service, err := selectItem(services, options.LogID)
			if err != nil {
				return nil, err
			}
			services = []models.Log{*service}
		}
		service, err := provider.SelectService(services)
		if err != nil {
			return nil, err
		}
		if service == nil || service.Actions == nil {
			return nil, errors.New("diagnostic provider selected no actionable log service")
		}
		action, err := findAction(service.Actions.Extra, layout.CollectAction)
		if err != nil {
			return nil, err
		}
		target = action.Target
		task.SourceLogServiceURI = models.Value(service.ODataID)
	}
	if target == "" {
		action, source, err := c.diagnosticAction(ctx, collectionURI, layout.CollectAction, options.LogID)
		if err != nil {
			return nil, err
		}
		target = action.Target
		task.SourceLogServiceURI = source
		if layout.ProgressSuffix != "" {
			task.ProgressURI = strings.TrimRight(target, "/") + layout.ProgressSuffix
		}
		if layout.ProbeBeforeCollect {
			previous, err := c.readDiagnosticProgress(ctx, provider, task.ProgressURI)
			if err != nil && !errors.Is(err, sdkerrors.ErrNotFound) {
				return nil, err
			}
			task.PreviousArtifact = previous.Artifact
		}
		if layout.DownloadAction != "" {
			download, _, err := c.diagnosticAction(ctx, collectionURI, layout.DownloadAction, options.LogID)
			if err != nil {
				return nil, err
			}
			task.DownloadURI = download.Target
			task.DownloadMethod = http.MethodPost
		}
		if layout.ProgressAction != "" {
			progress, _, err := c.diagnosticAction(ctx, collectionURI, layout.ProgressAction, options.LogID)
			if err != nil {
				return nil, err
			}
			task.ProgressURI = progress.Target
		}
	}

	response, err := c.Post(ctx, target, body)
	if err != nil {
		return nil, err
	}
	task.Response = response
	if validJSON(response.Body) {
		if err = response.Decode(task.Task); err != nil {
			return nil, err
		}
		var collection struct {
			Members []models.Link `json:"Members"`
		}
		if err = response.Decode(&collection); err != nil {
			return nil, err
		}
		if len(collection.Members) > 0 {
			task.Task.ODataID = collection.Members[0].ODataID
		}
	}
	task.Task.TaskMonitor = models.Ptr(response.Header.Get(wire.HeaderLocation))
	if layout.MonitorFromLocation {
		task.ProgressURI = response.Header.Get(wire.HeaderLocation)
		if task.ProgressURI == "" {
			task.ProgressURI = models.Value(task.Task.ODataID)
		}
		if task.ProgressURI == "" {
			return nil, errors.New("diagnostic collection does not advertise a job monitor")
		}
		monitor, err := c.resolve(task.ProgressURI, response.URL)
		if err != nil {
			return nil, err
		}
		task.ProgressURI = monitor.String()
	}

	if models.Value(task.Task.TaskState) == "" {
		task.Task.TaskState = models.Ptr(TaskRunning)
	}
	if artifact := response.Header.Get(wire.HeaderContentLocation); artifact != "" {
		task.Task.AdditionalDataURI = &artifact
	}
	return task, nil
}

func (p vendorLogCollect) Wait(ctx context.Context, c *Client, task *DiagnosticTask, options PollOptions) error {
	return c.waitDiagnostic(ctx, task, options)
}

func (c *Client) waitDiagnostic(ctx context.Context, task *DiagnosticTask, options PollOptions) error {
	if task == nil || task.Task == nil {
		return errors.New("diagnostic task metadata is missing")
	}
	strategy, err := c.diagnosticProvider(task.Vendor)
	if err != nil {
		return err
	}
	layout := diagnosticLayout(strategy)
	if layout.AllowTaskVisibilityDelay {
		options.AllowTaskVisibilityDelay = true
	}
	if options.Timeout == 0 {
		options.Timeout = 30 * time.Minute
	}
	if task.ProgressURI == "" {
		if models.Value(task.Task.TaskState) == TaskCompleted {
			return taskFailure(task.Task)
		}
		operation := &models.RedfishResponse{ODataID: task.Task.ODataID, TaskURI: task.Task.ODataID, TaskMonitor: task.Task.TaskMonitor}
		if task.Response != nil {
			operation.HTTPStatus = models.Ptr(int64(task.Response.StatusCode))
			operation.RetryAfter = models.Ptr(task.Response.Header.Get(wire.HeaderRetryAfter))
		}
		if models.Value(operation.TaskMonitor) == "" {
			operation.TaskMonitor = nil
		}
		if models.Value(operation.ODataID) == "" && models.Value(operation.TaskMonitor) == "" {
			if models.Value(task.Task.AdditionalDataURI) != "" {
				return nil
			}
			return errors.New("diagnostic collection returned no task or monitor reference")
		}
		result, err := c.WaitForOperation(ctx, operation, options)
		if err != nil {
			return err
		}
		if result.Response != nil {
			task.Response = result.Response
		}
		if result.Task != nil {
			if result.Task.ODataID == nil {
				result.Task.ODataID = task.Task.ODataID
			}
			if result.Task.Links == nil {
				result.Task.Links = task.Task.Links
			}
			if result.Task.Payload == nil {
				result.Task.Payload = task.Task.Payload
			}
			if result.Task.AdditionalDataURI == nil {
				result.Task.AdditionalDataURI = task.Task.AdditionalDataURI
			}
			task.Task = result.Task
		}
		return nil
	}
	ctx, cancel, interval, err := pollContext(ctx, options)
	if err != nil {
		return err
	}
	defer cancel()

	if task.Response != nil {
		if delay := retryDelay(task.Response.Header, 0, 0); delay > 0 {
			if err := waitContext(ctx, delay); err != nil {
				return err
			}
		}
	}
	observedNew := !task.FreshTrigger
	for iteration := 0; ; iteration++ {

		progress, err := c.readDiagnosticProgress(ctx, strategy, task.ProgressURI)
		if err != nil {
			return err
		}
		if !progress.Complete && !progress.Failed {
			observedNew = true
		}
		stale := layout.ProbeBeforeCollect && task.FreshTrigger && !observedNew && ((task.PreviousArtifact != "" && progress.Artifact == task.PreviousArtifact) || (task.PreviousArtifact == "" && iteration == 0))

		if progress.Complete && !stale && (!progress.Failed || progress.CompletionWins) {
			if validator, ok := diagnosticImplementation(strategy).(interface {
				ValidateArtifact(resourceprovider.DiagnosticProgress) error
			}); ok {
				if err := validator.ValidateArtifact(progress); err != nil {
					return err
				}
			}
			if progress.DownloadURI != "" {
				task.DownloadURI = progress.DownloadURI
				task.DownloadMethod = http.MethodGet
			}
			if progress.DownloadBody != nil {
				task.DownloadBody = progress.DownloadBody
			}
			task.Task.TaskState = models.Ptr(TaskCompleted)
			return nil
		}

		if progress.Failed && (!layout.ProbeBeforeCollect || !task.FreshTrigger || iteration > 0) {
			return &AsyncOperationError{Task: task.Task, Response: progress.Response, Message: redaction.String(progress.FailureMessage, c.passwordValue(), c.tokenSnapshot())}
		}
		delay := interval
		if progress.Response != nil {
			delay = retryDelay(progress.Response.Header, interval, 0)
		}
		if err = waitContext(ctx, delay); err != nil {
			return err
		}
	}
}

func (p vendorLogCollect) Download(ctx context.Context, c *Client, task *DiagnosticTask, path string) (string, error) {
	return c.downloadDiagnostic(ctx, task, path)
}

func (c *Client) DownloadDiagnosticData(ctx context.Context, taskOrEntry any, path string) (string, error) {
	switch item := taskOrEntry.(type) {
	case *DiagnosticTask:
		if item == nil {
			return "", errors.New("diagnostic task must not be nil")
		}
		strategy, err := c.diagnosticProvider(item.Vendor)
		if err != nil {
			return "", err
		}
		if provider, ok := strategy.(DiagnosticLifecycle); ok {
			return provider.Download(ctx, c, item, path)
		}
		return c.downloadDiagnostic(ctx, item, path)
	case *models.LogEntry:
		if item == nil {
			return "", errors.New("log entry must not be nil")
		}
		if models.Value(item.AdditionalDataURI) == "" {
			return "", errors.New("log entry is missing AdditionalDataURI")
		}
		return c.Download(ctx, models.Value(item.AdditionalDataURI), path)
	case *models.Task:
		if item == nil {
			return "", errors.New("task must not be nil")
		}
		return c.downloadDiagnostic(ctx, &DiagnosticTask{Task: item}, path)
	default:
		return "", errors.New("diagnostic download requires a DiagnosticTask, Task or LogEntry")
	}
}

type diagnosticArtifact struct {
	URI    string
	Method string
	Body   Object
}

func (c *Client) downloadDiagnostic(ctx context.Context, task *DiagnosticTask, path string) (string, error) {
	artifact, err := c.resolveDiagnosticArtifact(ctx, task)
	if err != nil {
		return "", err
	}
	if artifact.Method == http.MethodPost {
		return c.DownloadViaPost(ctx, artifact.URI, path, artifact.Body)
	}
	return c.Download(ctx, artifact.URI, path)
}

func (c *Client) resolveDiagnosticArtifact(ctx context.Context, task *DiagnosticTask) (*diagnosticArtifact, error) {
	if task.DownloadURI != "" {
		if task.DownloadMethod == http.MethodPost {
			body := task.DownloadBody
			if body == nil {
				body = Object{}
			}
			return &diagnosticArtifact{URI: task.DownloadURI, Method: http.MethodPost, Body: body}, nil
		}
		return &diagnosticArtifact{URI: task.DownloadURI, Method: http.MethodGet}, nil
	}
	if task.Task == nil {
		return nil, errors.New("diagnostic task metadata is missing")
	}
	if uri := taskArtifactURI(task); uri != "" {
		return &diagnosticArtifact{URI: uri, Method: http.MethodGet}, nil
	}
	if models.Value(task.Task.ODataID) != "" {
		fresh, err := getResource[models.Task](ctx, c, models.Value(task.Task.ODataID))
		if err != nil && !errors.Is(err, sdkerrors.ErrNotFound) {
			return nil, err
		}
		if fresh != nil {
			if fresh.Links != nil {
				task.Task.Links = fresh.Links
			}
			if fresh.Payload != nil {
				task.Task.Payload = fresh.Payload
			}
			if fresh.AdditionalDataURI != nil {
				task.Task.AdditionalDataURI = fresh.AdditionalDataURI
			}
			task.Task.Extra = fresh.Extra
			if uri := taskArtifactURI(task); uri != "" {
				return &diagnosticArtifact{URI: uri, Method: http.MethodGet}, nil
			}
		}
	}
	if uri := models.Value(task.Task.AdditionalDataURI); uri != "" {
		return &diagnosticArtifact{URI: uri, Method: http.MethodGet}, nil
	}
	if task.Task.Links != nil {
		for _, link := range task.Task.Links.CreatedResources {
			entry, err := linkedResource[models.LogEntry](ctx, c, &link, "CreatedResources")
			if err != nil {
				return nil, err
			}
			if models.Value(entry.AdditionalDataURI) != "" {
				return &diagnosticArtifact{URI: models.Value(entry.AdditionalDataURI), Method: http.MethodGet}, nil
			}
		}
	}
	for _, message := range task.Task.Messages {
		for _, arg := range message.MessageArgs {
			if strings.Contains(arg, "/Entries/") {
				entry, err := getResource[models.LogEntry](ctx, c, arg)
				if err != nil {
					return nil, err
				}
				if models.Value(entry.AdditionalDataURI) != "" {
					return &diagnosticArtifact{URI: models.Value(entry.AdditionalDataURI), Method: http.MethodGet}, nil
				}
			}
		}
	}
	if task.SourceLogServiceURI != "" {
		service, err := getResource[models.Log](ctx, c, task.SourceLogServiceURI)
		if err != nil {
			return nil, err
		}
		entries, err := linkedCollection[models.LogEntry](ctx, c, service.Entries, "Entries")
		if err != nil {
			return nil, err
		}
		slices.SortFunc(entries, func(a, b models.LogEntry) int {
			return strings.Compare(models.Value(b.Created), models.Value(a.Created))
		})
		for _, entry := range entries {
			if models.Value(entry.AdditionalDataURI) != "" {
				return &diagnosticArtifact{URI: models.Value(entry.AdditionalDataURI), Method: http.MethodGet}, nil
			}
		}
	}
	return nil, errors.New("diagnostic collection did not advertise a downloadable artifact")
}

func (c *Client) CollectAndDownloadDiagnosticData(ctx context.Context, path string, options CollectDownloadOptions) (string, error) {
	return collectAndDownloadDiagnostic(ctx, c, options, func(task *DiagnosticTask) (string, error) { return c.DownloadDiagnosticData(ctx, task, path) })
}

func collectAndDownloadDiagnostic[T any](ctx context.Context, c *Client, options CollectDownloadOptions, download func(*DiagnosticTask) (T, error)) (T, error) {
	var zero T
	if options.MaxRetries < 0 || options.RetryBackoff < 0 {
		return zero, errors.New("diagnostic retry options must not be negative")
	}
	var existing *DiagnosticTask
	if !options.DisableReuse {
		vendor, err := c.vendor(ctx)
		if err != nil {
			return zero, err
		}
		strategy, err := c.diagnosticProvider(vendor)
		if err != nil {
			return zero, err
		}
		if provider, ok := strategy.(interface {
			FindExisting(context.Context, *Client, DiagnosticOptions) (*DiagnosticTask, error)
		}); ok {
			existing, err = provider.FindExisting(ctx, c, options.Diagnostic)
			if err != nil {
				return zero, err
			}
		}
	}

	for attempt := 0; attempt <= options.MaxRetries; attempt++ {
		task := existing
		existing = nil
		var err error
		if task == nil {
			task, err = c.CollectDiagnosticData(ctx, options.Diagnostic)
			if err != nil {
				return zero, err
			}
		}
		strategy, loadErr := c.diagnosticProvider(task.Vendor)
		if loadErr != nil {
			return zero, loadErr
		}
		if provider, ok := strategy.(DiagnosticLifecycle); ok {
			err = provider.Wait(ctx, c, task, options.Poll)
		} else {
			err = c.waitDiagnostic(ctx, task, options.Poll)
		}
		if err == nil {
			return download(task)
		}
		var failed *AsyncOperationError
		if !errors.As(err, &failed) || attempt == options.MaxRetries {
			return zero, err
		}
		delay := options.RetryBackoff
		if delay == 0 {
			delay = 30 * time.Second
		}
		if err = waitContext(ctx, delay); err != nil {
			return zero, err
		}
	}
	return zero, errors.New("diagnostic attempts exhausted")
}

func (c *Client) collectSystemDiagnostic(ctx context.Context, provider LogCollectStrategy, options DiagnosticOptions) (*DiagnosticTask, error) {
	system, err := c.GetSystem(ctx)
	if err != nil {
		return nil, err
	}
	uri, err := requireLink(system.LogServices, "LogServices")
	if err != nil {
		return nil, err
	}
	action, source, err := c.diagnosticAction(ctx, uri, wire.ActionCollectDiagnosticData, options.LogID)
	if err != nil {
		return nil, err
	}
	body, err := provider.BuildBody(options)
	if err != nil {
		return nil, err
	}
	response, err := c.Post(ctx, action.Target, body)
	if err != nil {
		return nil, err
	}
	task := &models.Task{}
	if err = response.Decode(task); err != nil {
		return nil, err
	}
	if location := response.Header.Get(wire.HeaderLocation); location != "" {
		task.TaskMonitor = &location
	}
	return &DiagnosticTask{Task: task, Response: response, Vendor: VendorGeneric, SourceLogServiceURI: source, FreshTrigger: true}, nil
}

func (c *Client) WaitForDiagnosticData(ctx context.Context, task *DiagnosticTask, options PollOptions) error {
	if task == nil {
		return errors.New("diagnostic task must not be nil")
	}
	strategy, err := c.diagnosticProvider(task.Vendor)
	if err != nil {
		return err
	}
	if provider, ok := strategy.(DiagnosticLifecycle); ok {
		return provider.Wait(ctx, c, task, options)
	}
	return c.waitDiagnostic(ctx, task, options)
}

func taskTrackingURI(uri string) bool {
	return strings.Contains(uri, "/TaskService/Tasks/") || strings.Contains(uri, "/TaskService/TaskMonitors/")
}

func taskArtifactURI(task *DiagnosticTask) string {
	if task.Task == nil {
		return ""
	}
	if uri := models.Value(task.Task.AdditionalDataURI); uri != "" {
		return uri
	}
	if task.Response != nil {
		if uri := task.Response.Header.Get(wire.HeaderContentLocation); uri != "" && !taskTrackingURI(uri) {
			return uri
		}
	}
	if task.Task.Payload != nil {
		for _, header := range task.Task.Payload.HTTPHeaders {
			key, value, ok := strings.Cut(header, ":")
			if ok && (strings.EqualFold(strings.TrimSpace(key), wire.HeaderContentLocation) || strings.EqualFold(strings.TrimSpace(key), "Location")) {
				uri := strings.TrimSpace(value)
				if uri != "" && !taskTrackingURI(uri) {
					return uri
				}
			}
		}
	}
	var find func(map[string]json.RawMessage) string
	find = func(fields map[string]json.RawMessage) string {
		for _, key := range []string{"AdditionalDataURI", "DownloadURI", "DiagnosticDataURI"} {
			var uri string
			if json.Unmarshal(fields[key], &uri) == nil && uri != "" {
				return uri
			}
		}
		for _, raw := range fields {
			var nested map[string]json.RawMessage
			if json.Unmarshal(raw, &nested) == nil && nested != nil {
				if uri := find(nested); uri != "" {
					return uri
				}
			}
		}
		return ""
	}
	if uri := find(task.Task.Extra); uri != "" {
		return uri
	}
	if uri := find(task.Task.OEM); uri != "" {
		return uri
	}
	return ""
}

func (p vendorLogCollect) FindExisting(ctx context.Context, c *Client, options DiagnosticOptions) (*DiagnosticTask, error) {

	layout := diagnosticLayout(p)
	if layout.ReuseProgress {
		manager, err := c.GetManager(ctx, options.ManagerID)
		if err != nil {
			return nil, err
		}
		uri, err := requireLink(manager.LogServices, "LogServices")
		if err != nil {
			return nil, err
		}
		action, source, err := c.diagnosticAction(ctx, uri, layout.CollectAction, options.LogID)
		if err != nil {
			return nil, err
		}
		progressURI := strings.TrimRight(action.Target, "/") + layout.ProgressSuffix
		progress, err := c.readDiagnosticProgress(ctx, p, progressURI)
		if err != nil {
			if errors.Is(err, sdkerrors.ErrNotFound) {
				return nil, nil
			}
			return nil, err
		}
		if !progress.Reusable {
			return nil, nil
		}
		download, _, err := c.diagnosticAction(ctx, uri, layout.DownloadAction, options.LogID)
		if err != nil {
			return nil, err
		}
		return &DiagnosticTask{Task: &models.Task{TaskState: models.Ptr(TaskRunning)}, Vendor: p.vendor, ProgressURI: progressURI, DownloadURI: download.Target, DownloadMethod: http.MethodPost, SourceLogServiceURI: source}, nil
	}
	if !layout.ReuseTasks {
		return nil, nil
	}

	tasks, err := c.GetTasks(ctx)
	if err != nil {
		if errors.Is(err, sdkerrors.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	for _, task := range tasks {
		if models.Value(task.TaskState) != TaskRunning && models.Value(task.TaskState) != TaskPending {
			continue
		}
		name := strings.ToLower(models.Value(task.Name))
		target := ""
		if task.Payload != nil {
			target = models.Value(task.Payload.TargetURI)
		}
		if !strings.Contains(name, "diagnostic") && !strings.Contains(name, "log collect") && !strings.Contains(target, wire.ActionCollectDiagnosticData) && !strings.Contains(target, wire.ActionCollectAllLog) {
			continue
		}
		copy := task
		result := &DiagnosticTask{Vendor: p.vendor, Task: &copy}
		if layout.DownloadAction != "" {
			manager, err := c.GetManager(ctx, options.ManagerID)
			if err != nil {
				return nil, err
			}
			uri, err := requireLink(manager.LogServices, "LogServices")
			if err != nil {
				return nil, err
			}
			download, source, err := c.diagnosticAction(ctx, uri, layout.DownloadAction, options.LogID)
			if err != nil {
				return nil, err
			}
			result.DownloadURI = download.Target
			result.DownloadMethod = http.MethodPost
			result.SourceLogServiceURI = source
		}
		return result, nil
	}
	return nil, nil
}

func (c *Client) diagnosticProvider(vendor Vendor) (LogCollectStrategy, error) {
	if vendor == "" {
		vendor = VendorGeneric
	}
	return c.cfg.providers.ResolveLogCollect(vendor)
}

func diagnosticImplementation(strategy LogCollectStrategy) LogCollectStrategy {
	if adapter, ok := strategy.(vendorLogCollect); ok {
		return adapter.LogCollectStrategy
	}
	return strategy
}
func diagnosticLayout(strategy LogCollectStrategy) resourceprovider.DiagnosticLayout {
	if policy, ok := diagnosticImplementation(strategy).(resourceprovider.DiagnosticResourceStrategy); ok {
		return policy.DiagnosticLayout()
	}
	return (resourceprovider.DefaultLogCollectStrategy{}).DiagnosticLayout()
}
func (c *Client) readDiagnosticProgress(ctx context.Context, strategy LogCollectStrategy, uri string) (resourceprovider.DiagnosticProgress, error) {
	implementation := diagnosticImplementation(strategy)
	if reader, ok := implementation.(resourceprovider.DiagnosticResponseReader); ok {
		response, err := c.Request(ctx, http.MethodGet, uri, nil)
		if err != nil {
			return resourceprovider.DiagnosticProgress{}, err
		}
		progress, err := reader.DecodeDiagnosticResponse(response)
		progress.Response = response
		return progress, err
	}
	reader, ok := implementation.(resourceprovider.DiagnosticProgressReader)
	if !ok {
		return resourceprovider.DiagnosticProgress{}, errors.New("diagnostic provider does not support progress resources")
	}
	response, err := c.Request(ctx, http.MethodGet, uri, nil)
	if err != nil {
		return resourceprovider.DiagnosticProgress{}, err
	}
	progress, err := reader.DecodeProgress(response.Body)
	progress.Response = response
	return progress, err
}

func (c *Client) resolveArtifactReference(reference, base string) (string, error) {
	target, err := url.Parse(reference)
	if err != nil {
		return "", err
	}
	if !target.IsAbs() {
		origin, err := url.Parse(base)
		if err != nil {
			return "", err
		}
		target = origin.ResolveReference(target)
	}
	if _, err := validateArtifactURL(target.String()); err != nil {
		return "", err
	}
	return target.String(), nil
}
