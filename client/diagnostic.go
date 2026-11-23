package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	sdkerrors "github.com/rednote-infra/redfish-sdk-go/errors"
	"github.com/rednote-infra/redfish-sdk-go/internal/wire"
	"github.com/rednote-infra/redfish-sdk-go/models"
	"github.com/rednote-infra/redfish-sdk-go/pointer"
	sdkresponse "github.com/rednote-infra/redfish-sdk-go/response"
)

const (
	_ieitTaskVisibilityDelayCode = 17017
	_diagnosticManager           = "Manager"
	_diagnosticOEM               = "OEM"
	_diagnosticAllLogs           = "AllLogs"
	_h3cSDSPath                  = "/mnt/sdmmc0p1/sds/bmc/dump.tar.gz"
	_xfusionContentDirectory     = "/tmp/web"
	_diagnosticProtocolHTTPS     = wire.TransferProtocolHTTPS
)

type DiagnosticOptions struct {
	DiagnosticDataType, OEMDiagnosticDataType, LogID, ManagerID string
	OEMParameters                                               Object
}

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

type vendorLogCollect struct{ vendor Vendor }

func (p vendorLogCollect) BuildBody(options DiagnosticOptions) (Object, error) {
	kind := options.DiagnosticDataType
	body := Object{}
	switch p.vendor {
	case VendorInspur, VendorIEIT, VendorLenovo, VendorSmoothcompute:
	case VendorZTE:
		if kind == "" {
			kind = _diagnosticAllLogs
		}
		body[wire.FieldType] = kind
	case VendorH3C:
		body[wire.FieldType] = wire.DiagnosticOutputURI
		body[wire.FieldContent] = _h3cSDSPath
		if kind != "" {
			body["LogType"] = kind
		}
	case VendorXFusion:
		body[wire.FieldType] = wire.DiagnosticOutputURI
		body[wire.FieldContent] = fmt.Sprintf("%s/xf_sdk_%d.tar.gz", _xfusionContentDirectory, time.Now().UnixNano())
	case VendorEnginetech:
		if kind == "" {
			kind = _diagnosticOEM
		}
		body[wire.FieldDiagnosticDataType] = kind
		body[wire.FieldOEMDiagnosticDataType] = "BlackBox"
	default:
		if kind == "" {
			kind = _diagnosticManager
		}
		body[wire.FieldDiagnosticDataType] = kind
	}
	if options.OEMDiagnosticDataType != "" {
		body[wire.FieldOEMDiagnosticDataType] = options.OEMDiagnosticDataType
	}
	for key, value := range options.OEMParameters {
		body[key] = value
	}
	return body, nil
}

func (p vendorLogCollect) SelectService(services []models.Log) (*models.Log, error) {
	for _, service := range services {
		if service.Actions != nil {
			if _, err := findAction(service.Actions.Extra, wire.ActionCollectDiagnosticData); err == nil {
				return &service, nil
			}
		}
	}
	return nil, errors.New("no log service advertises CollectDiagnosticData")
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
			source = pointer.Value(service.ODataID)
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
	provider := c.cfg.providers.LogCollect(vendor)
	if lifecycle, ok := provider.(DiagnosticLifecycle); ok {
		return lifecycle.Collect(ctx, c, options)
	}
	return c.collectDiagnostic(ctx, vendor, provider, options)
}

func (p vendorLogCollect) Collect(ctx context.Context, c *Client, options DiagnosticOptions) (*DiagnosticTask, error) {
	return c.collectDiagnostic(ctx, p.vendor, p, options)
}

func (c *Client) collectDiagnostic(ctx context.Context, vendor Vendor, provider LogCollectStrategy, options DiagnosticOptions) (*DiagnosticTask, error) {
	if vendor == VendorEnginetech {
		return c.collectSystemDiagnostic(ctx, provider, options)
	}
	manager, err := c.GetManager(ctx, options.ManagerID)
	if err != nil {
		return nil, err
	}
	collectionURI, err := requireLink(manager.LogServices, "LogServices")
	if err != nil {
		return nil, err
	}
	task := &DiagnosticTask{Vendor: vendor, Task: &models.Task{TaskState: pointer.Ptr(TaskRunning)}, DownloadMethod: http.MethodGet, FreshTrigger: true}
	body, err := provider.BuildBody(options)
	if err != nil {
		return nil, err
	}
	var target string
	switch vendor {
	case VendorEnginetech:
		system, err := c.GetSystem(ctx)
		if err != nil {
			return nil, err
		}
		collectionURI, err = requireLink(system.LogServices, "LogServices")
		if err != nil {
			return nil, err
		}
	case VendorH3C, VendorXFusion:
		if manager.Actions == nil {
			return nil, errors.New("manager does not advertise diagnostic actions")
		}
		action, err := findAction(manager.Actions.Extra, wire.ActionDump)
		if err != nil && vendor == VendorXFusion {
			action, task.SourceLogServiceURI, err = c.diagnosticAction(ctx, collectionURI, "ExportLog", options.LogID)
		}
		if err != nil {
			return nil, err
		}
		target = action.Target
		download, err := findAction(manager.Actions.Extra, wire.ActionGeneralDownload)
		if err != nil {
			return nil, err
		}
		task.DownloadURI = download.Target
		task.DownloadMethod = http.MethodPost
		task.DownloadBody = Object{wire.FieldTransferProtocol: _diagnosticProtocolHTTPS}
		if vendor == VendorH3C {
			task.DownloadBody[wire.FieldContent] = _h3cSDSPath
		} else {
			task.DownloadBody[wire.FieldPath] = body[wire.FieldContent]
		}
	case VendorSmoothcompute:
		actions, err := c.resourceActions(ctx, strings.TrimRight(pointer.Value(manager.ODataID), "/")+"/DiagnosticService")
		if err != nil {
			return nil, err
		}
		action, err := findAction(actions, "CollectBlackInfo")
		if err != nil {
			return nil, err
		}
		target = action.Target
		download, err := findAction(actions, "DownloadBlackInfo")
		if err != nil {
			return nil, err
		}
		task.DownloadURI = download.Target
		task.DownloadMethod = http.MethodPost
	}
	if target == "" && vendor != VendorZTE && vendor != VendorInspur && vendor != VendorIEIT && vendor != VendorLenovo {
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
		action, err := findAction(service.Actions.Extra, wire.ActionCollectDiagnosticData)
		if err != nil {
			return nil, err
		}
		target = action.Target
		task.SourceLogServiceURI = pointer.Value(service.ODataID)
	}
	if target == "" {
		name := wire.ActionCollectDiagnosticData
		switch vendor {
		case VendorZTE:
			name = wire.ActionDump
		case VendorInspur, VendorIEIT, VendorLenovo:
			name = wire.ActionCollectAllLog
		}
		action, source, err := c.diagnosticAction(ctx, collectionURI, name, options.LogID)
		if err != nil {
			return nil, err
		}
		target = action.Target
		task.SourceLogServiceURI = source
		switch vendor {
		case VendorZTE:
			task.ProgressURI = strings.TrimRight(target, "/") + "/Progress"
			var previous zteProgress
			if err = c.Get(ctx, task.ProgressURI, &previous); err != nil && !errors.Is(err, sdkerrors.ErrNotFound) {
				return nil, err
			}
			task.PreviousArtifact = previous.TarPath
			download, _, err := c.diagnosticAction(ctx, collectionURI, wire.ActionExportLogs, options.LogID)
			if err != nil {
				return nil, err
			}
			task.DownloadURI = download.Target
			task.DownloadMethod = http.MethodPost
		case VendorInspur, VendorIEIT, VendorLenovo:
			download, _, err := c.diagnosticAction(ctx, collectionURI, wire.ActionDownloadAllLog, options.LogID)
			if err != nil {
				return nil, err
			}
			task.DownloadURI = download.Target
			task.DownloadMethod = http.MethodPost
			if vendor == VendorLenovo {
				progress, _, err := c.diagnosticAction(ctx, collectionURI, wire.ActionGetLogCollectProgress, options.LogID)
				if err != nil {
					return nil, err
				}
				task.ProgressURI = progress.Target
			}
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
	task.Task.TaskMonitor = pointer.Ptr(response.Header.Get(wire.HeaderLocation))
	if pointer.Value(task.Task.TaskState) == "" {
		task.Task.TaskState = pointer.Ptr(TaskRunning)
	}
	if artifact := response.Header.Get(wire.HeaderContentLocation); artifact != "" {
		task.Task.AdditionalDataURI = &artifact
	}
	return task, nil
}

type zteProgress struct {
	State      string                `json:"State"`
	Percentage models.StringOrNumber `json:"Percentage"`
	TarPath    string                `json:"TarPath"`
	Type       string                `json:"Type"`
}

type lenovoProgressFields struct {
	Progress *models.StringOrNumber `json:"Progress"`
	Status   *models.StringOrNumber `json:"Status"`
}

type lenovoProgress struct {
	OEM struct {
		Lenovo *lenovoProgressFields `json:"Lenovo"`
		Public *lenovoProgressFields `json:"Public"`
	} `json:"Oem"`
}

func (p vendorLogCollect) Wait(ctx context.Context, c *Client, task *DiagnosticTask, options PollOptions) error {
	return c.waitDiagnostic(ctx, task, options)
}

func (c *Client) waitDiagnostic(ctx context.Context, task *DiagnosticTask, options PollOptions) error {
	if task == nil || task.Task == nil {
		return errors.New("diagnostic task metadata is missing")
	}
	if task.Vendor == VendorIEIT {
		options.AllowTaskVisibilityDelay = true
	}
	if options.Timeout == 0 {
		options.Timeout = 30 * time.Minute
	}
	if task.ProgressURI == "" {
		if pointer.Value(task.Task.TaskState) == TaskCompleted {
			return taskFailure(task.Task)
		}
		operation := &models.RedfishResponse{ODataID: task.Task.ODataID, TaskURI: task.Task.ODataID, TaskMonitor: task.Task.TaskMonitor}
		if task.Response != nil {
			operation.HTTPStatus = pointer.Ptr(int64(task.Response.StatusCode))
			operation.RetryAfter = pointer.Ptr(task.Response.Header.Get(wire.HeaderRetryAfter))
		}
		if pointer.Value(operation.TaskMonitor) == "" {
			operation.TaskMonitor = nil
		}
		if pointer.Value(operation.ODataID) == "" && pointer.Value(operation.TaskMonitor) == "" {
			if pointer.Value(task.Task.AdditionalDataURI) != "" {
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
	observedNew := !task.FreshTrigger
	for iteration := 0; ; iteration++ {
		if task.Vendor == VendorZTE {
			var progress zteProgress
			if err = c.Get(ctx, task.ProgressURI, &progress); err != nil {
				return err
			}
			percentage, _ := strconv.Atoi(string(progress.Percentage))
			terminal := slices.Contains([]string{"STATE_COMPLETED", "STATE_SUCCESS", "STATE_SUCCESSED", "STATE_FINISH"}, progress.State) || percentage >= 100
			failed := progress.State == "STATE_FAILED" || progress.State == "STATE_ERROR"
			if !terminal && !failed {
				observedNew = true
			}
			stale := task.FreshTrigger && !observedNew && ((task.PreviousArtifact != "" && progress.TarPath == task.PreviousArtifact) || (task.PreviousArtifact == "" && iteration == 0))
			if terminal && !stale {
				if progress.TarPath == "" {
					return errors.New("ZTE collection completed without TarPath")
				}
				kind := progress.Type
				if kind == "" {
					kind = _diagnosticAllLogs
				}
				task.DownloadBody = Object{wire.FieldType: kind, wire.FieldContent: progress.TarPath}
				task.Task.TaskState = pointer.Ptr(TaskCompleted)
				return nil
			}
			if failed && (!task.FreshTrigger || iteration > 0) {
				return &AsyncOperationError{Task: task.Task, Message: "ZTE diagnostic collection failed: " + progress.State}
			}
		} else if task.Vendor == VendorLenovo {
			var progress lenovoProgress
			if err = c.Get(ctx, task.ProgressURI, &progress); err != nil {
				return err
			}
			fields := progress.OEM.Lenovo
			if fields == nil {
				fields = progress.OEM.Public
			}
			if fields == nil || fields.Progress == nil {
				return errors.New("Lenovo progress response is missing Progress")
			}
			percentage, err := strconv.Atoi(string(*fields.Progress))
			if err != nil {
				return err
			}
			if percentage >= 100 {
				if fields.Status != nil {
					status, err := strconv.Atoi(string(*fields.Status))
					if err != nil {
						return err
					}
					if status != 0 && status != 5 {
						return &AsyncOperationError{Task: task.Task, Message: "Lenovo diagnostic collection failed"}
					}
				}
				task.Task.TaskState = pointer.Ptr(TaskCompleted)
				return nil
			}
		}
		if err = waitContext(ctx, interval); err != nil {
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
		if provider, ok := c.cfg.providers.LogCollect(item.Vendor).(DiagnosticLifecycle); ok {
			return provider.Download(ctx, c, item, path)
		}
		return c.downloadDiagnostic(ctx, item, path)
	case *models.LogEntry:
		if item == nil {
			return "", errors.New("log entry must not be nil")
		}
		if pointer.Value(item.AdditionalDataURI) == "" {
			return "", errors.New("log entry is missing AdditionalDataURI")
		}
		return c.Download(ctx, pointer.Value(item.AdditionalDataURI), path)
	case *models.Task:
		if item == nil {
			return "", errors.New("task must not be nil")
		}
		return c.downloadDiagnostic(ctx, &DiagnosticTask{Task: item}, path)
	default:
		return "", errors.New("diagnostic download requires a DiagnosticTask, Task or LogEntry")
	}
}

func (c *Client) downloadDiagnostic(ctx context.Context, task *DiagnosticTask, path string) (string, error) {
	if task.DownloadURI != "" {
		if task.DownloadMethod == http.MethodPost {
			body := task.DownloadBody
			if body == nil {
				body = Object{}
			}
			return c.DownloadViaPost(ctx, task.DownloadURI, path, body)
		}
		return c.Download(ctx, task.DownloadURI, path)
	}
	if task.Task == nil {
		return "", errors.New("diagnostic task metadata is missing")
	}
	if uri := taskArtifactURI(task); uri != "" {
		return c.Download(ctx, uri, path)
	}
	if pointer.Value(task.Task.ODataID) != "" {
		fresh, err := getResource[models.Task](ctx, c, pointer.Value(task.Task.ODataID))
		if err != nil && !errors.Is(err, sdkerrors.ErrNotFound) {
			return "", err
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
				return c.Download(ctx, uri, path)
			}
		}
	}
	if uri := pointer.Value(task.Task.AdditionalDataURI); uri != "" {
		return c.Download(ctx, uri, path)
	}
	if task.Task.Links != nil {
		for _, link := range task.Task.Links.CreatedResources {
			entry, err := linkedResource[models.LogEntry](ctx, c, &link, "CreatedResources")
			if err != nil {
				return "", err
			}
			if pointer.Value(entry.AdditionalDataURI) != "" {
				return c.Download(ctx, pointer.Value(entry.AdditionalDataURI), path)
			}
		}
	}
	for _, message := range task.Task.Messages {
		for _, arg := range message.MessageArgs {
			if strings.Contains(arg, "/Entries/") {
				entry, err := getResource[models.LogEntry](ctx, c, arg)
				if err != nil {
					return "", err
				}
				if pointer.Value(entry.AdditionalDataURI) != "" {
					return c.Download(ctx, pointer.Value(entry.AdditionalDataURI), path)
				}
			}
		}
	}
	if task.SourceLogServiceURI != "" {
		service, err := getResource[models.Log](ctx, c, task.SourceLogServiceURI)
		if err != nil {
			return "", err
		}
		entries, err := linkedCollection[models.LogEntry](ctx, c, service.Entries, "Entries")
		if err != nil {
			return "", err
		}
		slices.SortFunc(entries, func(a, b models.LogEntry) int {
			return strings.Compare(pointer.Value(b.Created), pointer.Value(a.Created))
		})
		for _, entry := range entries {
			if pointer.Value(entry.AdditionalDataURI) != "" {
				return c.Download(ctx, pointer.Value(entry.AdditionalDataURI), path)
			}
		}
	}
	return "", errors.New("diagnostic collection did not advertise a downloadable artifact")
}

func (c *Client) CollectAndDownloadDiagnosticData(ctx context.Context, path string, options CollectDownloadOptions) (string, error) {
	if options.MaxRetries < 0 || options.RetryBackoff < 0 {
		return "", errors.New("diagnostic retry options must not be negative")
	}
	var existing *DiagnosticTask
	if !options.DisableReuse {
		vendor, err := c.vendor(ctx)
		if err != nil {
			return "", err
		}
		if provider, ok := c.cfg.providers.LogCollect(vendor).(interface {
			FindExisting(context.Context, *Client, DiagnosticOptions) (*DiagnosticTask, error)
		}); ok {
			existing, err = provider.FindExisting(ctx, c, options.Diagnostic)
			if err != nil {
				return "", err
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
				return "", err
			}
		}
		if provider, ok := c.cfg.providers.LogCollect(task.Vendor).(DiagnosticLifecycle); ok {
			err = provider.Wait(ctx, c, task, options.Poll)
		} else {
			err = c.waitDiagnostic(ctx, task, options.Poll)
		}
		if err == nil {
			return c.DownloadDiagnosticData(ctx, task, path)
		}
		var failed *AsyncOperationError
		if !errors.As(err, &failed) || attempt == options.MaxRetries {
			return "", err
		}
		delay := options.RetryBackoff
		if delay == 0 {
			delay = 30 * time.Second
		}
		if err = waitContext(ctx, delay); err != nil {
			return "", err
		}
	}
	return "", errors.New("diagnostic attempts exhausted")
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
	return &DiagnosticTask{Task: task, Response: response, Vendor: VendorEnginetech, SourceLogServiceURI: source, FreshTrigger: true}, nil
}

func (c *Client) WaitForDiagnosticData(ctx context.Context, task *DiagnosticTask, options PollOptions) error {
	if task == nil {
		return errors.New("diagnostic task must not be nil")
	}
	if provider, ok := c.cfg.providers.LogCollect(task.Vendor).(DiagnosticLifecycle); ok {
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
	if uri := pointer.Value(task.Task.AdditionalDataURI); uri != "" {
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
	if p.vendor == VendorZTE {
		manager, err := c.GetManager(ctx, options.ManagerID)
		if err != nil {
			return nil, err
		}
		uri, err := requireLink(manager.LogServices, "LogServices")
		if err != nil {
			return nil, err
		}
		action, source, err := c.diagnosticAction(ctx, uri, wire.ActionDump, options.LogID)
		if err != nil {
			return nil, err
		}
		progressURI := strings.TrimRight(action.Target, "/") + "/Progress"
		var progress zteProgress
		if err = c.Get(ctx, progressURI, &progress); err != nil {
			if errors.Is(err, sdkerrors.ErrNotFound) {
				return nil, nil
			}
			return nil, err
		}
		percent, err := strconv.Atoi(string(progress.Percentage))
		if err != nil {
			return nil, nil
		}
		if progress.State == "" || strings.Contains(progress.State, "FAILED") || strings.Contains(progress.State, "ERROR") || percent >= 100 || slices.Contains([]string{"STATE_COMPLETED", "STATE_SUCCESS", "STATE_SUCCESSED", "STATE_FINISH"}, progress.State) {
			return nil, nil
		}
		download, _, err := c.diagnosticAction(ctx, uri, wire.ActionExportLogs, options.LogID)
		if err != nil {
			return nil, err
		}
		return &DiagnosticTask{Task: &models.Task{TaskState: pointer.Ptr(TaskRunning)}, Vendor: p.vendor, ProgressURI: progressURI, DownloadURI: download.Target, DownloadMethod: http.MethodPost, SourceLogServiceURI: source}, nil
	}
	if p.vendor != VendorGeneric && p.vendor != VendorIEIT {
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
		if pointer.Value(task.TaskState) != TaskRunning && pointer.Value(task.TaskState) != TaskPending {
			continue
		}
		name := strings.ToLower(pointer.Value(task.Name))
		target := ""
		if task.Payload != nil {
			target = pointer.Value(task.Payload.TargetURI)
		}
		if !strings.Contains(name, "diagnostic") && !strings.Contains(name, "log collect") && !strings.Contains(target, wire.ActionCollectDiagnosticData) && !strings.Contains(target, wire.ActionCollectAllLog) {
			continue
		}
		copy := task
		result := &DiagnosticTask{Vendor: p.vendor, Task: &copy}
		if p.vendor == VendorIEIT {
			manager, err := c.GetManager(ctx, options.ManagerID)
			if err != nil {
				return nil, err
			}
			uri, err := requireLink(manager.LogServices, "LogServices")
			if err != nil {
				return nil, err
			}
			download, source, err := c.diagnosticAction(ctx, uri, wire.ActionDownloadAllLog, options.LogID)
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
