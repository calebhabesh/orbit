// Package controlclient selects one owner of state for CLI and terminal clients.
package controlclient

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/calebhabesh/file-sync/internal/app"
	"github.com/calebhabesh/file-sync/internal/config"
	"github.com/calebhabesh/file-sync/internal/control"
	tc "github.com/calebhabesh/file-sync/internal/control/terminalcontract"
	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/repository"
	"github.com/calebhabesh/file-sync/internal/state"
	"github.com/calebhabesh/file-sync/internal/workspace"
)

func PrivateFile(dir, name string, max int64) ([]byte, error) {
	return state.ReadPrivate(dir, name, max)
}

// Endpoint accepts numeric loopback only, without user info, paths or redirects.
func Endpoint(address string) (string, error) {
	if !strings.Contains(address, "://") {
		address = "http://" + address
	}
	u, err := url.Parse(address)
	if err != nil || u.Scheme != "http" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("invalid local control endpoint")
	}
	ip := net.ParseIP(u.Hostname())
	port := u.Port()
	number, e := strconv.ParseUint(port, 10, 16)
	if e != nil || number == 0 || ip == nil || !ip.IsLoopback() {
		return "", errors.New("control endpoint must be numeric loopback with a port")
	}
	return u.String(), nil
}

type Client struct {
	StateDir      string
	RestartDaemon func(context.Context) error
}

var _ tc.Client = (*Client)(nil)

// Call is the bounded compatibility API used by existing CLI helpers.
func (c *Client) Call(ctx context.Context, method, path string, input, output any) error {
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") {
		return errors.New("invalid control path")
	}
	addr, err := PrivateFile(c.StateDir, "control.addr", 4096)
	if err != nil {
		return err
	}
	endpoint, err := Endpoint(strings.TrimSpace(string(addr)))
	if err != nil {
		return err
	}
	token, err := PrivateFile(c.StateDir, "control.token", 4096)
	if err != nil {
		return err
	}
	credential := strings.TrimSpace(string(token))
	if credential == "" || strings.ContainsAny(credential, "\r\n") {
		return errors.New("invalid local credential")
	}
	var body io.Reader
	if input != nil {
		b, err := json.Marshal(input)
		if err != nil {
			return err
		}
		if len(b) > tc.MaxMetadata {
			return errors.New("PAYLOAD_TOO_LARGE")
		}
		body = bytes.NewReader(b)
	}
	// The daemon answers terminal calls under one lock that its setup worker
	// holds for up to an 8 s slice (app.go), and a setup/adopt/join mutation
	// advances its job for up to 30 s before replying. Wait past those bounds
	// so a slow enrollment round trip is not reported as a failure.
	callTimeout, headerTimeout := 15*time.Second, 12*time.Second
	switch v := input.(type) {
	case tc.Query:
		if v.Kind == "network_doctor" {
			callTimeout, headerTimeout = 25*time.Second, 25*time.Second
		}
	case tc.Mutation:
		if v.Kind == "setup" || v.Kind == "adopt" || v.Kind == "join" {
			callTimeout, headerTimeout = 40*time.Second, 35*time.Second
		}
	}
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, endpoint+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+credential)
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	transport := &http.Transport{Proxy: nil, DisableCompression: true, DialContext: (&net.Dialer{Timeout: 2 * time.Second}).DialContext, ResponseHeaderTimeout: headerTimeout, MaxResponseHeaderBytes: 16 << 10}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("control redirects are forbidden") }}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	cfg, err := config.Load(c.StateDir)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 && resp.Header.Get("X-Orbit-Device") != cfg.DeviceID {
		return errors.New("IDENTITY_MISMATCH: local daemon state")
	}

	b, err := io.ReadAll(io.LimitReader(resp.Body, tc.MaxMetadata+1))
	if err != nil {
		return err
	}
	if len(b) > tc.MaxMetadata {
		return errors.New("PAYLOAD_TOO_LARGE")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var e control.ControlError
		if json.Unmarshal(b, &e) == nil && e.Code != "" {
			return &e
		}
		return fmt.Errorf("control HTTP %d", resp.StatusCode)
	}
	if output != nil {
		if _, ok := output.(*tc.Result); ok {
			return tc.Decode(b, output)
		}
		return json.Unmarshal(b, output)
	}
	return nil
}

// WithController never interprets a live HTTP error as permission to open SQLite.
func (c *Client) WithController(ctx context.Context, live func() error, stopped func(*control.Controller) error) error {
	entered := false
	err := app.WithWorkspace(ctx, c.StateDir, func(cfg config.Config, db *repository.DB, ws *workspace.Workspace) error {
		entered = true
		var id history.ID
		decoded, err := decodeID(cfg.DeviceID)
		if err != nil {
			return err
		}
		copy(id[:], decoded)
		return stopped(control.New(db, ws, control.Options{LocalDevice: id, StoppedAdapter: true}))
	})
	if !entered && errors.Is(err, state.ErrLocked) {
		return live()
	}
	return err
}
func decodeID(s string) ([]byte, error) {
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != 32 {
		return nil, errors.New("invalid device identity")
	}
	return b, nil
}

func (c *Client) Query(ctx context.Context, q tc.Query) (r tc.Result, err error) {
	if err = q.Validate(); err != nil {
		return r, err
	}
	err = c.WithController(ctx, func() error { return c.terminalCall(ctx, "/query", q, &r) }, func(ctrl *control.Controller) error { r, err = ctrl.TerminalQuery(ctx, q); return err })
	return
}
func (c *Client) Mutate(ctx context.Context, m tc.Mutation) (r tc.Result, err error) {
	if err = m.Validate(); err != nil {
		return r, err
	}
	err = c.WithController(ctx, func() error { return c.terminalCall(ctx, "/mutate", m, &r) }, func(ctrl *control.Controller) error { r, err = ctrl.TerminalMutate(ctx, m); return err })
	if err == nil && m.Kind == "service" && r.Operation != nil && r.Operation.State == "running" {
		owned, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		claim := struct {
			Granted bool `json:"granted"`
		}{false}
		err = c.WithController(owned, func() error {
			return c.Call(owned, http.MethodPost, "/control/terminal/v1/service/claim", control.TerminalServiceCompletion{ID: m.OperationID}, &claim)
		}, func(ctrl *control.Controller) error {
			claim.Granted, err = ctrl.ClaimTerminalService(owned, m.OperationID)
			return err
		})
		if err != nil {
			return r, err
		}
		if !claim.Granted {
			return c.Query(owned, tc.Query{Version: tc.Version, Kind: "operation", ID: m.OperationID})
		}
		actual, actionErr := control.ExecuteTerminalService(owned, c.StateDir, *m.Service)
		completion := control.TerminalServiceCompletion{ID: m.OperationID}
		if actionErr == nil && actual != nil && !actual.Success {
			actionErr = errors.New("requested service state was not observed")
		}
		if actionErr != nil {
			var e *control.ControlError
			if errors.As(actionErr, &e) {
				completion.Failure = e
			} else {
				completion.Failure = &control.ControlError{Code: "IO_ERROR", Message: actionErr.Error(), Action: "inspect actual service state"}
			}
		}
		err = c.WithController(owned, func() error {
			return c.Call(owned, http.MethodPost, "/control/terminal/v1/service/complete", completion, &r)
		}, func(ctrl *control.Controller) error {
			r, err = ctrl.CompleteTerminalService(owned, completion)
			return err
		})
	}

	return
}
func (c *Client) terminalCall(ctx context.Context, path string, input any, out *tc.Result) error {
	var capabilities tc.Result
	if q, ok := input.(tc.Query); !ok || q.Kind != "capabilities" {
		if err := c.Call(ctx, http.MethodPost, "/control/terminal/v1/query", tc.Query{Version: tc.Version, Kind: "capabilities"}, &capabilities); err != nil {
			return err
		}
		if capabilities.Version != tc.Version {
			return &control.ControlError{Code: "INCOMPATIBLE_VERSION", Message: "unsupported terminal control version", Action: "use a compatible daemon"}
		}
		if !slices.Contains(capabilities.Capabilities, tc.Capability) {
			return &control.ControlError{Code: "UNSUPPORTED_CAPABILITY", Message: "daemon does not advertise terminal control", Action: "use a compatible daemon"}
		}
	}
	if q, ok := input.(tc.Query); ok && (q.Kind == "setups" || q.Kind == "folder_management") && !slices.Contains(capabilities.Capabilities, "onboarding_management_v1") {
		return &control.ControlError{Code: "UNSUPPORTED_CAPABILITY", Message: "daemon does not advertise onboarding management", Action: "use a compatible daemon"}
	}
	needsNetwork := false
	switch v := input.(type) {
	case tc.Mutation:
		needsNetwork = v.Kind == "network"
	case tc.Query:
		needsNetwork = v.Kind == "network_status" || v.Kind == "network_preview" || v.Kind == "network_doctor"
	}
	if needsNetwork && !slices.Contains(capabilities.Capabilities, tc.NetworkCapability) {
		return &control.ControlError{Code: "UNSUPPORTED_CAPABILITY", Message: "daemon does not advertise network controls", Action: "use a compatible daemon"}
	}
	if q, ok := input.(tc.Query); ok && q.Kind == "network_doctor" && !slices.Contains(capabilities.Capabilities, tc.NetworkDiagnosticsCapability) {
		return &control.ControlError{Code: "UNSUPPORTED_CAPABILITY", Message: "daemon does not advertise explicit network diagnostics", Action: "use a compatible daemon"}
	}
	needsContent := false
	switch v := input.(type) {
	case tc.Mutation:
		needsContent = v.Kind == "content" || v.Kind == "session" || v.Kind == "cancel"
	case tc.Query:
		needsContent = v.Kind == "conflicts" || v.Kind == "content_review" || v.Kind == "session" || v.Kind == "history" || v.Kind == "deleted"
	}
	if needsContent && !slices.Contains(capabilities.Capabilities, "reviewed_content_v1") {
		return &control.ControlError{Code: "UNSUPPORTED_CAPABILITY", Message: "daemon does not advertise reviewed content workflows", Action: "use a compatible daemon"}
	}

	return c.Call(ctx, http.MethodPost, "/control/terminal/v1"+path, input, out)
}
