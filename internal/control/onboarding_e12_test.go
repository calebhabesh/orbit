package control

import (
	"context"
	"encoding/hex"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/config"
	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/calebhabesh/orbit/internal/protocol"
	"github.com/calebhabesh/orbit/internal/replication"
	"github.com/calebhabesh/orbit/internal/repository"
)

func TestOnboardingE12GlobalRequestCountIgnoresPagingAndExpiry(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()
	if err := config.Save(env.stateDir, config.Config{FormatVersion: config.FormatVersion, DeviceID: hex.EncodeToString(env.authorA[:]), CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	if err := env.db.RecordEnrollmentRequest(ctx, repository.EnrollmentRequestRecord{RequestID: "legacy", Folder: env.folder, DeviceID: env.authorB, Status: "pending"}); err != nil {
		t.Fatal(err)
	}
	now := env.ctrl.options.Now()
	put := func(key string, expires time.Time, state string) {
		t.Helper()
		record := replication.EnrollmentRecord{Expires: expires.Unix(), Wire: protocol.TerminalEnrollmentWire{Folder: hex.EncodeToString(env.folder[:]), Attempt: key, Label: "Laptop"}, Result: protocol.TerminalEnrollmentResult{State: state}}
		if err := env.db.EnrollmentTransaction(ctx, func(tx *repository.EnrollmentTx) error { return tx.Put("request/"+key, record) }); err != nil {
			t.Fatal(err)
		}
	}
	put("fresh", now.Add(time.Hour), "pending_approval")
	put("expired", now.Add(-time.Hour), "pending_approval")
	put("completed", now.Add(time.Hour), "approved")
	for _, kind := range []string{"capabilities", "folders", "devices", "requests"} {
		r, err := env.ctrl.TerminalQuery(ctx, tc.Query{Version: tc.Version, Kind: kind, Limit: 1})
		if err != nil || r.JoinRequestCount == nil || *r.JoinRequestCount != 2 {
			t.Fatalf("%s: missing global count independent of selected page: %v, %v", kind, r.JoinRequestCount, err)
		}
	}
	if err := env.db.UpdateEnrollmentRequestStatus(ctx, "legacy", "approved"); err != nil {
		t.Fatal(err)
	}
	put("fresh", now.Add(time.Hour), "approved")
	r, err := env.ctrl.TerminalQuery(ctx, tc.Query{Version: tc.Version, Kind: "capabilities"})
	if err != nil || r.JoinRequestCount == nil || *r.JoinRequestCount != 0 {
		t.Fatal("approved or expired requests remain in the badge", r.JoinRequestCount, err)
	}
}
