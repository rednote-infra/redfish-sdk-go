package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/rednote-infra/redfish-sdk-go/models"
)

// DiagnosticBytesDownloader adds in-memory artifact downloads to a custom lifecycle.
type DiagnosticBytesDownloader interface {
	DownloadBytes(context.Context, *Client, *DiagnosticTask) ([]byte, error)
}

// DownloadBytesViaPost supports vendors whose download action requires a JSON POST.
func (c *Client) DownloadBytesViaPost(ctx context.Context, uri string, body any) ([]byte, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	response, err := c.downloadResponse(ctx, http.MethodPost, uri, bytes.NewReader(data), "application/json")
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	return io.ReadAll(response.Body)
}

func (p vendorLogCollect) DownloadBytes(ctx context.Context, c *Client, task *DiagnosticTask) ([]byte, error) {
	return c.downloadDiagnosticBytes(ctx, task)
}

func (c *Client) downloadDiagnosticBytes(ctx context.Context, task *DiagnosticTask) ([]byte, error) {
	artifact, err := c.resolveDiagnosticArtifact(ctx, task)
	if err != nil {
		return nil, err
	}
	if artifact.Method == http.MethodPost {
		return c.DownloadBytesViaPost(ctx, artifact.URI, artifact.Body)
	}
	return c.DownloadBytes(ctx, artifact.URI)
}

// DownloadDiagnosticDataBytes downloads a task or log-entry artifact without writing a file.
func (c *Client) DownloadDiagnosticDataBytes(ctx context.Context, taskOrEntry any) ([]byte, error) {
	switch item := taskOrEntry.(type) {
	case *DiagnosticTask:
		if item == nil {
			return nil, errors.New("diagnostic task must not be nil")
		}
		provider, err := c.diagnosticProvider(item.Vendor)
		if err != nil {
			return nil, err
		}
		if memory, ok := provider.(DiagnosticBytesDownloader); ok {
			return memory.DownloadBytes(ctx, c, item)
		}
		return c.downloadDiagnosticBytes(ctx, item)
	case *models.Task:
		if item == nil {
			return nil, errors.New("task must not be nil")
		}
		return c.downloadDiagnosticBytes(ctx, &DiagnosticTask{Task: item})
	case *models.LogEntry:
		if item == nil || models.Value(item.AdditionalDataURI) == "" {
			return nil, errors.New("log entry is missing AdditionalDataURI")
		}
		return c.DownloadBytes(ctx, models.Value(item.AdditionalDataURI))
	default:
		return nil, errors.New("diagnostic download requires a DiagnosticTask, Task or LogEntry")
	}
}

// CollectAndDownloadDiagnosticDataBytes runs collection, polling, and download in memory.
func (c *Client) CollectAndDownloadDiagnosticDataBytes(ctx context.Context, options CollectDownloadOptions) ([]byte, error) {
	return collectAndDownloadDiagnostic(ctx, c, options, func(task *DiagnosticTask) ([]byte, error) { return c.DownloadDiagnosticDataBytes(ctx, task) })
}
