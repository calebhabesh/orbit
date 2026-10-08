package controlclient

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/calebhabesh/orbit/internal/control"
	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
)

// A device that has never been set up reports NOT_SET_UP with a next step,
// not a raw filesystem error.
func TestQueryWithoutStateReportsNotSetUp(t *testing.T) {
	c := &Client{StateDir: filepath.Join(t.TempDir(), "missing")}
	_, err := c.Query(context.Background(), tc.Query{Version: tc.Version, Kind: "status"})
	var ce *control.ControlError
	if !errors.As(err, &ce) || ce.Code != "NOT_SET_UP" || ce.Action == "" {
		t.Fatalf("expected NOT_SET_UP control error, got %v", err)
	}
}
