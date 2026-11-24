package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"

	sdkerrors "github.com/rednote-infra/redfish-sdk-go/errors"
	"github.com/rednote-infra/redfish-sdk-go/internal/wire"
	sdkresponse "github.com/rednote-infra/redfish-sdk-go/response"
)

func (c *Client) streamOnce(ctx context.Context, method, uri string, body io.Reader, contentType string) (*http.Response, error) {
	c.mu.RLock()
	closed := c.closed
	c.mu.RUnlock()
	if closed {
		return nil, sdkerrors.ErrClosed
	}
	target, err := c.resolve(uri, c.base.String()+"/")
	if err != nil {
		return nil, err
	}
	if c.authenticationMode() == AuthenticationSession && c.tokenSnapshot() == "" {
		if err = c.Login(ctx); err != nil {
			return nil, err
		}
	}
	request, err := http.NewRequestWithContext(ctx, method, target.String(), body)
	if err != nil {
		return nil, err
	}
	request.Header.Set(wire.HeaderODataVersion, wire.ODataVersion)
	if token := c.tokenSnapshot(); token != "" {
		request.Header.Set(wire.HeaderAuthToken, token)
	} else {
		username, password := c.credentials()
		request.SetBasicAuth(username, password)
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	response, err := c.http.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, &sdkerrors.TransportError{Method: method, URI: target.EscapedPath(), Cause: err}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		defer response.Body.Close()
		data, _ := io.ReadAll(io.LimitReader(response.Body, _maxJSONBytes))
		return nil, sdkerrors.NewHTTPError(&sdkresponse.Response{StatusCode: response.StatusCode, Header: response.Header, Body: data, URL: target.String()}, c.passwordValue(), c.tokenSnapshot())
	}
	return response, nil
}

func (c *Client) Download(ctx context.Context, uri, outputPath string) (string, error) {
	response, err := c.downloadResponse(ctx, http.MethodGet, uri, nil, "")
	if err != nil {
		return "", err
	}
	return saveDownload(response, outputPath)
}

func (c *Client) DownloadViaPost(ctx context.Context, uri, outputPath string, body any) (string, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	response, err := c.downloadResponse(ctx, http.MethodPost, uri, bytes.NewReader(data), "application/json")
	if err != nil {
		return "", err
	}
	return saveDownload(response, outputPath)
}

func saveDownload(response *http.Response, outputPath string) (path string, err error) {
	defer response.Body.Close()
	if outputPath == "" {
		_, params, parseErr := mime.ParseMediaType(response.Header.Get("Content-Disposition"))
		filename := "diagnostic.tar.gz"
		if parseErr == nil && params["filename"] != "" {
			candidate := filepath.Base(strings.ReplaceAll(params["filename"], "\\", "/"))
			if candidate != "." && candidate != ".." && candidate != "/" {
				filename = candidate
			}
		}
		outputPath = filename
	}
	info, statErr := os.Stat(outputPath)
	if statErr == nil && info.IsDir() {
		return "", errors.New("output path must name a file")
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0700); err != nil {
		return "", err
	}
	temporary, err := os.CreateTemp(filepath.Dir(outputPath), ".redfish-download-*")
	if err != nil {
		return "", err
	}
	temporaryPath := temporary.Name()
	defer func() {
		temporary.Close()
		if err != nil {
			os.Remove(temporaryPath)
		}
	}()
	if _, err = io.Copy(temporary, response.Body); err != nil {
		return "", err
	}
	if err = temporary.Sync(); err != nil {
		return "", err
	}
	if err = temporary.Close(); err != nil {
		return "", err
	}
	if err = os.Rename(temporaryPath, outputPath); err != nil {
		return "", err
	}
	return outputPath, nil
}

func (c *Client) uploadMultipart(ctx context.Context, uri, filePath, field string, fields map[string]json.RawMessage) (*sdkresponse.Response, error) {
	if _, err := c.resolve(uri, c.base.String()+"/"); err != nil {
		return nil, err
	}
	if strings.TrimSpace(field) == "" {
		return nil, errors.New("multipart file field must not be empty")
	}
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("firmware upload requires a regular file")
	}
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	reader, writer := io.Pipe()
	form := multipart.NewWriter(writer)
	contentType := form.FormDataContentType()
	done := make(chan error, 1)
	go func() {
		writeErr := func() error {
			for name, data := range fields {
				header := textproto.MIMEHeader{}
				header.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{"name": name}))
				header.Set("Content-Type", "application/json")
				part, err := form.CreatePart(header)
				if err != nil {
					return err
				}
				if _, err = part.Write(data); err != nil {
					return err
				}
			}
			part, err := form.CreateFormFile(field, filepath.Base(filePath))
			if err != nil {
				return err
			}
			if _, err = io.Copy(part, file); err != nil {
				return err
			}
			return form.Close()
		}()
		writer.CloseWithError(writeErr)
		done <- writeErr
	}()
	response, err := c.stream(ctx, http.MethodPost, uri, reader, contentType)
	reader.Close()
	writeErr := <-done
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if writeErr != nil {
		return nil, writeErr
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, _maxJSONBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > _maxJSONBytes {
		return nil, errors.New("upload response exceeds size limit")
	}
	return &sdkresponse.Response{StatusCode: response.StatusCode, Header: response.Header.Clone(), Body: data, URL: uri}, nil
}

func (c *Client) stream(ctx context.Context, method, uri string, body io.Reader, contentType string) (*http.Response, error) {
	token := c.tokenSnapshot()
	response, err := c.streamOnce(ctx, method, uri, body, contentType)
	if sdkerrors.IsStatus(err, http.StatusUnauthorized) && safeMethod(method) && c.authenticationMode() == AuthenticationSession {
		if err = c.refresh(ctx, token); err != nil {
			return nil, err
		}
		return c.streamOnce(ctx, method, uri, nil, contentType)
	}
	return response, err
}
