package terminalcontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/calebhabesh/file-sync/internal/history"
)

func validID(s string) bool {
	b, err := hex.DecodeString(s)
	return err == nil && len(b) == 32 && s == strings.ToLower(s) && s != strings.Repeat("0", 64)
}
func validReview(r Review) bool {
	_, err := time.Parse(time.RFC3339Nano, r.ExpiresAt)
	return validID(r.Token) && validID(r.Generation) && err == nil
}
func validSettings(s Settings) bool {
	return s.DataBudget > 0 && s.MetadataBudget > 0 && s.ReserveBytes > 0 && s.Concurrency > 0 && s.Concurrency <= 32 && slices.Contains([]string{"manual", "login", "unattended"}, s.Startup)
}
func validName(s string) bool {
	return s != "" && len(s) <= 256 && utf8.ValidString(s) && !strings.ContainsRune(s, 0)
}
func validVersion(v VersionID, folder string) bool {
	return validID(v.Folder) && v.Folder == folder && validID(v.Author) && v.Counter > 0
}
func validHeads(vs []VersionID, folder string) bool {
	if len(vs) > 64 {
		return false
	}
	var last string
	for _, v := range vs {
		if !validVersion(v, folder) {
			return false
		}
		// Numeric counter ordering, padded to fixed uint64 decimal width.
		key := fmt.Sprintf("%s:%020d", v.Author, v.Counter)
		if key <= last {
			return false
		}
		last = key
	}
	return true
}
func (m Mutation) Validate() error {
	if m.Version != Version {
		return fmt.Errorf("INCOMPATIBLE_VERSION")
	}
	if !validID(m.OperationID) {
		return fmt.Errorf("INVALID_REQUEST: operation identity")
	}
	count := 0
	for _, present := range []bool{m.Network != nil, m.Setup != nil, m.Invite != nil, m.Join != nil, m.Approval != nil, m.Folder != nil, m.Content != nil, m.Session != nil, m.Settings != nil, m.Service != nil, m.Cancel != nil} {
		if present {
			count++
		}
	}
	if count != 1 {
		return fmt.Errorf("INVALID_REQUEST: exactly one intent required")
	}
	ok := false
	switch m.Kind {
	case "setup", "adopt":
		p := m.Setup
		ok = p != nil && validName(p.DeviceName) && validName(p.FolderName) && filepath.IsAbs(p.Root) && validReview(p.Preview) && validSettings(p.Settings)
	case "invite", "share":
		p := m.Invite
		ok = p != nil && validID(p.Folder) && validID(p.ExpectedMembership) && (p.Device == "" || validID(p.Device))
		if ok {
			_, err := time.Parse(time.RFC3339Nano, p.ExpiresAt)
			ok = err == nil && (m.Kind != "share" || validID(p.Device))
		}
	case "join":
		p := m.Join
		ok = p != nil && validID(p.Attempt) && validName(p.DeviceName) && validName(p.FolderName) && filepath.IsAbs(p.Root) && validReview(p.Preview) && validSettings(p.Settings)
		if ok {
			ok = p.Invitation.Validate() == nil
		}
	case "approval":
		p := m.Approval
		ok = p != nil && validID(p.Request) && validID(p.Folder) && validID(p.Requester) && validID(p.KeyPin) && validID(p.TranscriptDigest) && validID(p.ExpectedMembership) && slices.Contains([]string{"approve", "decline"}, p.Decision)
	case "folder":
		p := m.Folder
		ok = p != nil && validID(p.Folder) && validReview(p.Review) && slices.Contains([]string{"pause", "resume", "relocate"}, p.Action)
		if ok && p.Action == "relocate" {
			ok = filepath.IsAbs(p.ExpectedRoot) && filepath.IsAbs(p.Destination)
		}
	case "content":
		p := m.Content
		ok = p != nil && validID(p.Context.Folder) && validID(p.Context.Generation) && history.ValidatePath(p.Context.Path) == nil && validReview(p.Review) && validHeads(p.Heads, p.Context.Folder)
		if ok {
			switch p.Action {
			case "select":
				ok = validVersion(p.Source, p.Context.Folder) && slices.Contains(p.Heads, p.Source)
			case "restore":
				ok = validVersion(p.Source, p.Context.Folder)
			case "separate_copy":
				ok = validVersion(p.Source, p.Context.Folder) && history.ValidatePath(p.Destination) == nil && p.Destination != p.Context.Path && validReview(p.DestinationReview)
			case "merge":
				ok = validID(p.Session) && validID(p.Upload) && validID(p.Digest)
			case "keep_copies":
				ok = len(p.Copies) > 0 && len(p.Copies) <= 64
				seen := map[string]bool{}
				for _, c := range p.Copies {
					if !slices.Contains(p.Heads, c.Source) || history.ValidatePath(c.Destination) != nil || c.Destination == p.Context.Path || !validReview(c.Review) || seen[c.Destination] {
						ok = false
					}
					seen[c.Destination] = true
				}
			default:
				ok = false
			}
		}
	case "session":
		p := m.Session
		ok = p != nil && slices.Contains([]string{"create", "renew", "discard"}, p.Action) && (p.Action == "create" || validID(p.ID)) && validID(p.Context.Folder) && validID(p.Context.Generation) && history.ValidatePath(p.Context.Path) == nil && validReview(p.Review) && validHeads(p.Heads, p.Context.Folder) && len(p.Sources) > 0 && len(p.Sources) <= 64
		if ok {
			for _, v := range p.Sources {
				if !validVersion(v, p.Context.Folder) {
					ok = false
				}
			}
		}
	case "network":
		p := m.Network
		ok = p != nil && validReview(p.Review) && p.Policy.Validate() == nil && len(p.ServiceRoots) <= MaxServiceRoots && (p.ServiceRoots == "" || (p.Profile != nil && p.Environment != "release"))
	case "settings":
		p := m.Settings
		ok = p != nil && validReview(p.Review) && validSettings(p.Settings)
	case "service":
		p := m.Service
		ok = p != nil && validReview(p.Review) && slices.Contains([]string{"start", "stop", "restart", "enable", "disable"}, p.Action) && slices.Contains([]string{"manual", "login", "unattended"}, p.Mode)
	case "cancel":
		ok = m.Cancel != nil && validID(m.Cancel.Target) && m.Cancel.Target != m.OperationID
	}
	if ok && m.Setup != nil && m.Setup.Network != nil {
		ok = m.Setup.Network.Validate() == nil
	}
	if ok && m.Join != nil && m.Join.Network != nil {
		ok = m.Join.Network.Validate() == nil
	}
	if !ok {
		return fmt.Errorf("INVALID_REQUEST: invalid %s intent", m.Kind)
	}
	return nil
}

// Fingerprint binds normalized typed inputs and domain, excluding only the replay
// identity. Arrays with set semantics must already be in canonical order.
func (m Mutation) Fingerprint() (string, error) {
	if err := m.Validate(); err != nil {
		return "", err
	}
	m.OperationID = ""
	b, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(append([]byte("orbit-terminal-mutation-v1\x00"), b...))
	return hex.EncodeToString(sum[:]), nil
}
func (q Query) Validate() error {
	if q.Version != Version {
		return fmt.Errorf("INCOMPATIBLE_VERSION")
	}
	if q.Limit > MaxPage {
		return fmt.Errorf("INVALID_REQUEST: page limit")
	}
	if q.Folder != "" && !validID(q.Folder) {
		return fmt.Errorf("INVALID_REQUEST: folder")
	}
	if q.Path != "" && q.Kind != "root_preview" && history.ValidatePath(q.Path) != nil {
		return fmt.Errorf("INVALID_PATH")
	}
	switch q.Kind {
	case "network_doctor":
		if q.ID != "" && !validID(q.ID) {
			return fmt.Errorf("INVALID_REQUEST: device")
		}
	case "network_status", "network_preview", "capabilities", "context", "status", "attention", "devices", "folders", "requests", "settings", "service", "doctor":
	case "root_preview":
		if q.RootPlan != nil && (q.RootPlan.Root != q.Path || !validName(q.RootPlan.DeviceName) || !validName(q.RootPlan.FolderName) || !validSettings(q.RootPlan.Settings)) {
			return fmt.Errorf("INVALID_REQUEST: root plan")
		}
		if !filepath.IsAbs(q.Path) {
			return fmt.Errorf("INVALID_REQUEST: root path")
		}
	case "setups":
		if q.Folder != "" || q.Name != "" || q.ID != "" || q.Path != "" || q.Cwd != "" || q.RootPlan != nil || (q.Cursor != "" && !validID(q.Cursor)) {
			return fmt.Errorf("INVALID_REQUEST: unfiltered setup page")
		}
	case "storage":
	case "paths", "maintenance":
		if !validID(q.Folder) || len(q.Name) > 256 {
			return fmt.Errorf("INVALID_REQUEST: folder/search")
		}
	case "folder_management":
		if !validID(q.Folder) {
			return fmt.Errorf("INVALID_REQUEST: folder required")
		}
	case "operation", "session":
		if !validID(q.ID) {
			return fmt.Errorf("INVALID_REQUEST: identity")
		}
	case "history", "deleted", "content_review", "conflicts":
		if !validID(q.Folder) {
			return fmt.Errorf("INVALID_REQUEST: folder required")
		}
		if q.Kind != "deleted" && q.Kind != "conflicts" && q.Path == "" {
			return fmt.Errorf("INVALID_PATH")
		}
		if q.Source != nil && !validVersion(*q.Source, q.Folder) {
			return fmt.Errorf("INVALID_REQUEST: source")
		}
	default:
		return fmt.Errorf("INVALID_REQUEST: query kind")
	}
	return nil
}
func (r Readiness) Ready() bool {
	return r.Approved && r.MembershipCurrent && r.RootAvailable && r.ScanComplete && r.Unsupported == 0 && r.Unreadable == 0 && r.Uncaptured == 0 && r.MissingContent == 0 && r.PendingPublication == 0 && r.Conflicts == 0 && !r.StorageBlocked
}

// Exit categories are stable across human and JSON renderers.
const (
	ExitOK             = 0
	ExitFailure        = 1
	ExitInvalid        = 2
	ExitAuthentication = 3
	ExitReview         = 4
	ExitPending        = 5
	ExitResource       = 6
	ExitIncompatible   = 7
	ExitCanceled       = 130
)

func ExitCode(r Result) int {
	if r.Error != nil {
		switch r.Error.Code {
		case "INVALID_REQUEST", "INVALID_PATH", "AMBIGUOUS_CONTEXT", "FOLDER_NOT_FOUND", "PAYLOAD_TOO_LARGE":
			return ExitInvalid
		case "UNAUTHORIZED", "IDENTITY_MISMATCH", "INVALID_SIGNATURE", "INVITATION_EXPIRED", "INVITATION_REVOKED":
			return ExitAuthentication
		case "STALE_VIEW", "STALE_ROOT", "DESTINATION_COLLISION", "MEMBERSHIP_FORK", "IDEMPOTENCY_CONFLICT", "EXPIRED_REPLAY", "CONTENT_EXPIRED":
			return ExitReview
		case "OFFLINE", "CONTENT_PENDING", "CONTENT_UNAVAILABLE", "ROOT_UNAVAILABLE", "RATE_LIMITED", "DAEMON_REQUIRED", "SERVICE_UNAVAILABLE", "PROFILE_MISSING", "PROFILE_EXPIRED":
			return ExitPending
		case "DISK_BUDGET", "METADATA_BUDGET":
			return ExitResource
		case "INCOMPATIBLE_VERSION", "UNSUPPORTED_CAPABILITY", "LOCAL_DISCOVERY_NOT_IMPLEMENTED", "LOCAL_ONLY_ROUTE_UNAVAILABLE":
			return ExitIncompatible
		case "CANCELED":
			return ExitCanceled
		default:
			return ExitFailure
		}
	}
	switch r.State {
	case "loading", "awaiting_approval", "offline", "pending", "running", "blocked":
		return ExitPending
	case "partial", "fork", "stale", "conflict":
		return ExitReview
	case "canceled":
		return ExitCanceled
	case "failed":
		return ExitFailure
	case "storage_blocked":
		return ExitResource
	case "root_unavailable":
		return ExitPending
	}
	return ExitOK
}

func (u UploadIntent) Fingerprint() (string, error) {
	if !validID(u.OperationID) || !validID(u.Session) || !validID(u.Digest) || u.Bytes > 64<<30 {
		return "", fmt.Errorf("INVALID_REQUEST: upload")
	}
	u.OperationID = ""
	b, err := json.Marshal(u)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(append([]byte("orbit-terminal-upload-v1\x00"), b...))
	return hex.EncodeToString(sum[:]), nil
}
