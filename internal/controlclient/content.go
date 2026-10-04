package controlclient

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/calebhabesh/file-sync/internal/config"
	"github.com/calebhabesh/file-sync/internal/control"
	tc "github.com/calebhabesh/file-sync/internal/control/terminalcontract"
)

// streamCall authenticates to the same private numeric-loopback endpoint as
// metadata calls. The caller's context owns the stream lifetime.
func (c *Client) streamCall(ctx context.Context, path string, metadata any, body io.Reader) (*http.Response, func(), error) {
	var caps tc.Result
	if err := c.Call(ctx, http.MethodPost, "/control/terminal/v1/query", tc.Query{Version: tc.Version, Kind: "capabilities"}, &caps); err != nil {
		return nil, nil, err
	}
	if caps.Version != tc.Version || !slices.Contains(caps.Capabilities, tc.Capability) || !slices.Contains(caps.Capabilities, "reviewed_content_v1") {
		return nil, nil, &control.ControlError{Code: "UNSUPPORTED_CAPABILITY", Message: "daemon does not advertise reviewed content streams", Action: "use a compatible daemon"}
	}
	addr, err := PrivateFile(c.StateDir, "control.addr", 4096)
	if err != nil {
		return nil, nil, err
	}
	endpoint, err := Endpoint(strings.TrimSpace(string(addr)))
	if err != nil {
		return nil, nil, err
	}
	token, err := PrivateFile(c.StateDir, "control.token", 4096)
	if err != nil {
		return nil, nil, err
	}
	credential := strings.TrimSpace(string(token))
	if credential == "" || strings.ContainsAny(credential, "\r\n") {
		return nil, nil, errors.New("invalid local credential")
	}
	data, err := json.Marshal(metadata)
	if err != nil || len(data) > tc.MaxMetadata {
		return nil, nil, errors.New("INVALID_REQUEST: stream metadata")
	}
	// A bounded metadata header leaves the request body entirely for bytes.
	if len(data) > 8192 {
		return nil, nil, errors.New("PAYLOAD_TOO_LARGE")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+path, body)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+credential)
	req.Header.Set("X-Orbit-Intent", string(data))
	req.Header.Set("Content-Type", "application/octet-stream")
	transport := &http.Transport{Proxy: nil, DisableCompression: true, DialContext: (&net.Dialer{Timeout: 2 * time.Second}).DialContext, MaxResponseHeaderBytes: 16 << 10}
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("control redirects are forbidden") }}
	cleanup := transport.CloseIdleConnections
	resp, err := client.Do(req)
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	fail := func(err error) (*http.Response, func(), error) { resp.Body.Close(); cleanup(); return nil, nil, err }
	cfg, err := config.Load(c.StateDir)
	if err != nil {
		return fail(err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, err := io.ReadAll(io.LimitReader(resp.Body, tc.MaxMetadata+1))
		if err != nil {
			return fail(err)
		}
		var e control.ControlError
		if json.Unmarshal(b, &e) == nil && e.Code != "" {
			return fail(&e)
		}
		return fail(errors.New("control stream HTTP error"))
	}
	if resp.Header.Get("X-Orbit-Device") != cfg.DeviceID {
		return fail(errors.New("IDENTITY_MISMATCH: local daemon state"))
	}
	return resp, cleanup, nil
}

type ownedRead struct {
	io.ReadCloser
	cleanup func()
}

func (r *ownedRead) Close() error { err := r.ReadCloser.Close(); r.cleanup(); return err }

func (c *Client) Read(ctx context.Context, intent tc.ReadIntent) (io.ReadCloser, error) {
	// Validate exact identity before touching state; the server validates ranges.
	if intent.Version.Counter == 0 || len(intent.Version.Folder) != 64 || len(intent.Version.Author) != 64 {
		return nil, errors.New("INVALID_REQUEST: exact version")
	}
	streamCtx, cancel := context.WithCancel(ctx)
	ready := make(chan error, 1)
	done := make(chan struct{})
	reader, writer := io.Pipe()
	var liveRead io.ReadCloser
	go func() {
		defer close(done)
		announced := false
		announce := func(err error) { announced = true; ready <- err }
		err := c.WithController(streamCtx, func() error {
			resp, cleanup, err := c.streamCall(streamCtx, "/control/terminal/v1/read", intent, nil)
			if err != nil {
				announce(err)
				return err
			}
			liveRead = &ownedRead{ReadCloser: resp.Body, cleanup: cleanup}
			announce(nil)
			// Keep the owner goroutine alive until Close/cancellation.
			<-streamCtx.Done()
			liveRead.Close()
			return streamCtx.Err()
		}, func(ctrl *control.Controller) error {
			r, err := ctrl.ExactRead(streamCtx, intent)
			if err != nil {
				announce(err)
				return err
			}
			defer r.Close()
			announce(nil)
			_, err = io.CopyBuffer(writer, r, make([]byte, 64<<10))
			return err
		})
		if !announced {
			ready <- err
		}
		writer.CloseWithError(err)
	}()
	select {
	case err := <-ready:
		if err != nil {
			cancel()
			reader.Close()
			<-done
			return nil, err
		}
		if liveRead != nil {
			reader.Close()
			return &ownedRead{ReadCloser: liveRead, cleanup: func() { cancel(); <-done }}, nil
		}
		return &ownedRead{ReadCloser: reader, cleanup: func() { cancel(); <-done }}, nil
	case <-ctx.Done():
		cancel()
		reader.Close()
		<-done
		return nil, ctx.Err()
	}
}
func (c *Client) Upload(ctx context.Context, intent tc.UploadIntent, source io.Reader) (r tc.Result, err error) {
	if _, err = intent.Fingerprint(); err != nil {
		return r, err
	}
	err = c.WithController(ctx, func() error {
		resp, cleanup, e := c.streamCall(ctx, "/control/terminal/v1/upload", intent, source)
		if e != nil {
			return e
		}
		defer cleanup()
		defer resp.Body.Close()
		data, e := io.ReadAll(io.LimitReader(resp.Body, tc.MaxMetadata+1))
		if e != nil {
			return e
		}
		if len(data) > tc.MaxMetadata {
			return errors.New("PAYLOAD_TOO_LARGE")
		}
		return tc.Decode(data, &r)
	}, func(ctrl *control.Controller) error { r, err = ctrl.TerminalUpload(ctx, intent, source); return err })
	return
}
