// Package terminalcontract freezes the additive terminal v1 control boundary.
// It defines intent/observation codecs, not engine orchestration or authorization.
// T02–T12 supply adapters and durable production implementation.
package terminalcontract

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/calebhabesh/orbit/internal/network"
	"io"
	"strconv"

	"github.com/calebhabesh/orbit/internal/protocol"
)

const Version = "1"

// DefaultOrbitName names a new Orbit (and its folder, ~/Synced) when the
// owner gives no name; it reads as what the folder is, not as the app.
const DefaultOrbitName = "Synced"
const Capability = "terminal_control_v1"
const MaxMetadata = 1 << 20
const MaxPage = 200

// Uint is always a canonical decimal JSON string, including values above 2^53.
type Uint uint64

func (n Uint) MarshalJSON() ([]byte, error) { return json.Marshal(strconv.FormatUint(uint64(n), 10)) }
func (n *Uint) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil || strconv.FormatUint(v, 10) != s {
		return fmt.Errorf("noncanonical uint64 %q", s)
	}
	*n = Uint(v)
	return nil
}

// Decode reuses the peer strict codec. Semantic request checks are explicit.
func Decode(b []byte, out any) error {
	if len(b) > MaxMetadata {
		return fmt.Errorf("PAYLOAD_TOO_LARGE")
	}
	return protocol.DecodeStrict(b, out)
}

type VersionID struct {
	Folder  string `json:"folder"`
	Author  string `json:"author"`
	Counter Uint   `json:"counter"`
}
type Context struct {
	Folder     string `json:"folder"`
	FolderName string `json:"folder_name"`
	Root       string `json:"root"`
	Path       string `json:"path"`
	Generation string `json:"generation"`
}
type Review struct {
	Token      string `json:"token"`
	Generation string `json:"generation"`
	ExpiresAt  string `json:"expires_at"`
}
type Error struct {
	Code              string `json:"code"`
	Message           string `json:"message"`
	Retryable         bool   `json:"retryable"`
	Action            string `json:"action"`
	RetryAfterSeconds Uint   `json:"retry_after_seconds"`
}
type Settings struct {
	DataBudget              Uint   `json:"data_budget"`
	MetadataBudget          Uint   `json:"metadata_budget"`
	ReserveBytes            Uint   `json:"reserve_bytes"`
	RetentionSeconds        Uint   `json:"retention_seconds"`
	Concurrency             Uint   `json:"concurrency"`
	BandwidthBytesPerSecond Uint   `json:"bandwidth_bytes_per_second"`
	PeerListen              string `json:"peer_listen"`
	EnrollmentListen        string `json:"enrollment_listen"`
	AdvertisedPeer          string `json:"advertised_peer"`
	AdvertisedEnrollment    string `json:"advertised_enrollment"`
	Startup                 string `json:"startup"` // manual, login, unattended
}
type RootPreview struct {
	Review             Review  `json:"review"`
	Root               string  `json:"root"`
	Registration       string  `json:"registration"`
	Device             Uint    `json:"device"`
	Inode              Uint    `json:"inode"`
	Files              Uint    `json:"files"`
	Directories        Uint    `json:"directories"`
	Bytes              Uint    `json:"bytes"`
	Unsupported        Uint    `json:"unsupported"`
	Unreadable         Uint    `json:"unreadable"`
	Complete           bool    `json:"complete"`
	CapacityKnown      bool    `json:"capacity_known"`
	AvailableBytes     Uint    `json:"available_bytes"`
	CapacityObservedAt string  `json:"capacity_observed_at"`
	Missing            bool    `json:"missing,omitempty"` // created, with missing parents, on confirmation
	Issues             []Issue `json:"issues"`
	Cursor             string  `json:"cursor"`
}
type Issue struct {
	Path string `json:"path"`
	Code string `json:"code"`
}
type SetupIntent struct {
	Network    *NetworkPolicy `json:"network,omitempty"`
	DeviceName string         `json:"device_name"`
	FolderName string         `json:"folder_name"`
	Root       string         `json:"root"`
	Preview    Review         `json:"preview"`
	Settings   Settings       `json:"settings"`
}
type Invitation struct {
	Route              *protocol.EnrollmentRoute `json:"route,omitempty"`
	Profile            *protocol.NetworkProfile  `json:"profile,omitempty"`
	Version            string                    `json:"version"`
	Folder             string                    `json:"folder"`
	Inviter            string                    `json:"inviter"`
	CertificateDER     string                    `json:"certificate_der"` // base64 DER; exact trust anchor
	KeyPin             string                    `json:"key_pin"`         // SHA-256 SPKI
	EnrollmentEndpoint string                    `json:"enrollment_endpoint"`
	PeerEndpoint       string                    `json:"peer_endpoint"`
	Capability         string                    `json:"capability"` // explicit transfer only; never in status
	ExpiresAt          string                    `json:"expires_at"`
	// Display names chosen on the inviting device, shown to the joiner and
	// proposed as its local folder name. Not identity: trust is the key pin.
	InviterName string `json:"inviter_name,omitempty"`
	FolderName  string `json:"folder_name,omitempty"`
}
type InviteIntent struct {
	ShortCode          bool   `json:"short_code,omitempty"`
	Folder             string `json:"folder"`
	ExpectedMembership string `json:"expected_membership"`
	ExpiresAt          string `json:"expires_at"`
	Device             string `json:"device"` // empty for a new device; exact known key for sharing
}

// PairingIntent authorizes one ephemeral short-code claim through the exact
// operator/profile whose privacy text the client showed before submission.
type PairingIntent struct {
	Code    string `json:"code"`
	Profile string `json:"profile"`
}
type JoinIntent struct {
	Network    *NetworkPolicy `json:"network,omitempty"`
	Invitation Invitation     `json:"invitation"`
	Attempt    string         `json:"attempt"` // fresh random 32 bytes hex, persisted before network
	DeviceName string         `json:"device_name"`
	FolderName string         `json:"folder_name"`
	Root       string         `json:"root"`
	Preview    Review         `json:"preview"`
	Settings   Settings       `json:"settings"`
}
type ApprovalIntent struct {
	Request            string `json:"request"`
	Folder             string `json:"folder"`
	Requester          string `json:"requester"`
	KeyPin             string `json:"key_pin"`
	TranscriptDigest   string `json:"transcript_digest"`
	ExpectedMembership string `json:"expected_membership"`
	Decision           string `json:"decision"` // approve, decline
}
type FolderIntent struct {
	Folder       string `json:"folder"`
	Review       Review `json:"review"`
	Action       string `json:"action"` // pause, resume, relocate
	ExpectedRoot string `json:"expected_root"`
	Destination  string `json:"destination"`
}
type ReadIntent struct {
	Version VersionID `json:"version"`
	Offset  Uint      `json:"offset"`
	Length  Uint      `json:"length"`
}
type ContentIntent struct {
	Context           Context     `json:"context"`
	Review            Review      `json:"review"`
	Heads             []VersionID `json:"heads"`
	Source            VersionID   `json:"source"`
	Action            string      `json:"action"` // select, keep_copies, merge, restore, separate_copy
	Session           string      `json:"session"`
	Upload            string      `json:"upload"`
	Digest            string      `json:"digest"`
	Bytes             Uint        `json:"bytes"`
	Destination       string      `json:"destination"`
	DestinationReview Review      `json:"destination_review"`
	Copies            []CopyPlan  `json:"copies"`
}
type CopyPlan struct {
	Source      VersionID `json:"source"`
	Destination string    `json:"destination"`
	Review      Review    `json:"review"`
}
type SessionIntent struct {
	Action  string      `json:"action"`
	ID      string      `json:"id"`
	Context Context     `json:"context"`
	Review  Review      `json:"review"`
	Heads   []VersionID `json:"heads"`
	Sources []VersionID `json:"sources"`
}
type EditorSession struct {
	ID         string      `json:"id"`
	Context    Context     `json:"context"`
	Review     Review      `json:"review"`
	Heads      []VersionID `json:"heads"`
	Sources    []VersionID `json:"sources"`
	ResultPath string      `json:"result_path"`
	ExpiresAt  string      `json:"expires_at"`
	State      string      `json:"state"` // active, expired, recovery, closed
}
type SettingsIntent struct {
	Review   Review   `json:"review"`
	Settings Settings `json:"settings"`
}
type ServiceIntent struct {
	Action string `json:"action"` // start, stop, restart, enable, disable
	Mode   string `json:"mode"`   // manual, login, unattended
	Review Review `json:"review"`
}

// Mutation is a closed tagged union. Exactly one matching intent is required.
// Operation IDs are random 32-byte lowercase hex, scoped to this state identity.
type Mutation struct {
	Pairing     *PairingIntent  `json:"pairing,omitempty"`
	Network     *NetworkIntent  `json:"network,omitempty"`
	Version     string          `json:"version"`
	OperationID string          `json:"operation_id"`
	Kind        string          `json:"kind"`
	Setup       *SetupIntent    `json:"setup,omitempty"`
	Invite      *InviteIntent   `json:"invite,omitempty"`
	Join        *JoinIntent     `json:"join,omitempty"`
	Approval    *ApprovalIntent `json:"approval,omitempty"`
	Folder      *FolderIntent   `json:"folder,omitempty"`
	Content     *ContentIntent  `json:"content,omitempty"`
	Session     *SessionIntent  `json:"session,omitempty"`
	Settings    *SettingsIntent `json:"settings,omitempty"`
	Service     *ServiceIntent  `json:"service,omitempty"`
	Cancel      *CancelIntent   `json:"cancel,omitempty"`
}
type CancelIntent struct {
	Target string `json:"target"`
}
type Query struct {
	NetworkPlan *NetworkIntent `json:"network_plan,omitempty"`
	RootPlan    *SetupIntent   `json:"root_plan,omitempty"`
	Version     string         `json:"version"`
	Kind        string         `json:"kind"` // capabilities, context, root_preview, operation, status,
	// attention, devices, folders, requests, history, deleted, content_review,
	// session, settings, service, doctor, files, file_details
	Folder      string     `json:"folder"`
	Path        string     `json:"path"`
	Cwd         string     `json:"cwd"`
	Name        string     `json:"name"`
	ID          string     `json:"id"`
	Source      *VersionID `json:"source,omitempty"`
	Destination string     `json:"destination"`
	Limit       Uint       `json:"limit"`
	Cursor      string     `json:"cursor"`
}
type Operation struct {
	ID               string   `json:"id"`
	Fingerprint      string   `json:"fingerprint"`
	Kind             string   `json:"kind"`
	State            string   `json:"state"` // pending, running, blocked, completed, partial, canceled, failed
	Phase            string   `json:"phase"`
	CommittedEffects []Effect `json:"committed_effects"`
	CancelRequested  bool     `json:"cancel_requested"`
	Error            *Error   `json:"error,omitempty"`
}
type Effect struct {
	Path    string     `json:"path"`
	Version *VersionID `json:"version,omitempty"`
	Source  *VersionID `json:"source,omitempty"`
	State   string     `json:"state"`
}
type Readiness struct {
	Approved           bool `json:"approved"`
	MembershipCurrent  bool `json:"membership_current"`
	RootAvailable      bool `json:"root_available"`
	ScanComplete       bool `json:"scan_complete"`
	Unsupported        Uint `json:"unsupported"`
	Unreadable         Uint `json:"unreadable"`
	Uncaptured         Uint `json:"uncaptured"`
	MissingContent     Uint `json:"missing_content"`
	PendingPublication Uint `json:"pending_publication"`
	Conflicts          Uint `json:"conflicts"`
	StorageBlocked     bool `json:"storage_blocked"`
}
type JoinRecord struct {
	Operation          Operation `json:"operation"`
	Attempt            string    `json:"attempt"`
	Request            string    `json:"request"`
	Folder             string    `json:"folder"`
	Root               string    `json:"root"`
	Preview            Review    `json:"preview"`
	Inviter            string    `json:"inviter"`
	KeyPin             string    `json:"key_pin"`
	CertificateDER     string    `json:"certificate_der"`
	EnrollmentEndpoint string    `json:"enrollment_endpoint"`
	PeerEndpoint       string    `json:"peer_endpoint"`
	Readiness          Readiness `json:"readiness"`
}
type Observation struct {
	DeviceName   string    `json:"device_name,omitempty"`
	Device       string    `json:"device"`
	Folder       string    `json:"folder"`
	Version      VersionID `json:"version"`
	Saved        bool      `json:"saved"`
	Stored       bool      `json:"stored"`
	Applied      bool      `json:"applied"`
	Direct       bool      `json:"direct"`
	ObservedAt   string    `json:"observed_at"`
	LastContact  string    `json:"last_contact"`
	Availability string    `json:"availability"` // available, pending, expired, unavailable, corrupt, unknown
	Online       bool      `json:"online"`
}
type Attention struct {
	ID          string `json:"id"`
	Folder      string `json:"folder"`
	Path        string `json:"path"`
	Code        string `json:"code"`
	Action      string `json:"action"`
	OperationID string `json:"operation_id"`
}
type Service struct {
	Running            bool   `json:"running"`
	Enabled            bool   `json:"enabled"`
	Mode               string `json:"mode"`
	UnattendedVerified bool   `json:"unattended_verified"`
	RootHealthy        bool   `json:"root_healthy"`
	CaptureHealthy     bool   `json:"capture_healthy"`
	// Owner names who runs the daemon (service, terminal, manual), separate
	// from the configured startup Mode; UnitState is systemd's ActiveState.
	Owner     string `json:"owner,omitempty"`
	UnitState string `json:"unit_state,omitempty"`
}

// HostStartup is the advisory startup default for this host. Orbit never
// enables lingering itself; LingerCommand is shown for the owner to run.
type HostStartup struct {
	Class         string `json:"class"`     // desktop, headless, unknown
	Suggested     string `json:"suggested"` // manual, login, unattended
	Systemd       bool   `json:"systemd"`
	Lingering     bool   `json:"lingering"`
	LingerCommand string `json:"linger_command,omitempty"`
	Note          string `json:"note,omitempty"`
}
type NamedItem struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Root   string `json:"root"`
	KeyPin string `json:"key_pin"`
}
type EnrollmentRequest struct {
	ID                 string `json:"id"`
	Folder             string `json:"folder"`
	Requester          string `json:"requester"`
	KeyPin             string `json:"key_pin"`
	Label              string `json:"label"`
	TranscriptDigest   string `json:"transcript_digest"`
	VerificationCode   string `json:"verification_code"`
	ExpectedMembership string `json:"expected_membership"`
	State              string `json:"state"`
}

// Result is also the fixture/view vocabulary. Loading is a client observation,
// never a successful engine mutation. Collections are [] rather than null.
type FolderManagement struct {
	Name             string      `json:"name"`
	LocalDevice      string      `json:"local_device"`
	RemovedBy        string      `json:"removed_by,omitempty"`
	RemovalReporter  string      `json:"removal_reporter,omitempty"`
	Root             string      `json:"root"`
	Paused           bool        `json:"paused"`
	MembershipDigest string      `json:"membership_digest"`
	Revision         Uint        `json:"revision"`
	Members          []NamedItem `json:"members"`
}

type Storage struct {
	Objects        Uint `json:"objects"`
	Metadata       Uint `json:"metadata"`
	Staging        Uint `json:"staging"`
	Recovery       Uint `json:"recovery"`
	Quarantine     Uint `json:"quarantine"`
	DataBudget     Uint `json:"data_budget"`
	MetadataBudget Uint `json:"metadata_budget"`
	Reserve        Uint `json:"reserve"`
}

type RetirementReview struct {
	DeviceName       string `json:"device_name"`
	ReceivedChanges  int    `json:"received_changes"`
	MembershipDigest string `json:"membership_digest"`
	SnapshotDigest   string `json:"snapshot_digest"`
	Warning          string `json:"warning"`
	Disclaimer       string `json:"disclaimer"`
}

type RemovalResult struct {
	DeviceID        string   `json:"device_id"`
	State           string   `json:"state"`
	OperationID     string   `json:"operation_id"`
	ReceivedChanges int      `json:"received_changes"`
	PendingDevices  []string `json:"pending_devices"`
	Message         string   `json:"message"`
}

type Result struct {
	Retirement       *RetirementReview      `json:"retirement,omitempty"`
	Removal          *RemovalResult         `json:"removal,omitempty"`
	Pairing          *network.PairingStatus `json:"pairing,omitempty"`
	Network          *NetworkStatus         `json:"network,omitempty"`
	CopyPlans        []CopyPlan             `json:"copy_plans,omitempty"`
	Storage          *Storage               `json:"storage,omitempty"`
	FolderManagement *FolderManagement      `json:"folder_management,omitempty"`
	Upload           *UploadResult          `json:"upload,omitempty"`
	Versions         []VersionSummary       `json:"versions"`
	ContentReview    *ContentReview         `json:"content_review,omitempty"`
	Version          string                 `json:"version"`
	Capabilities     []string               `json:"capabilities"`
	State            string                 `json:"state"`
	Context          *Context               `json:"context,omitempty"`
	Operation        *Operation             `json:"operation,omitempty"`
	Preview          *RootPreview           `json:"preview,omitempty"`
	Join             *JoinRecord            `json:"join,omitempty"`
	Invitation       *Invitation            `json:"invitation,omitempty"`
	Session          *EditorSession         `json:"session,omitempty"`
	Settings         *Settings              `json:"settings,omitempty"`
	Service          *Service               `json:"service,omitempty"`
	Host             *HostStartup           `json:"host,omitempty"`
	Readiness        *Readiness             `json:"readiness,omitempty"`
	Items            []NamedItem            `json:"items"`
	Requests         []EnrollmentRequest    `json:"requests"`
	Observations     []Observation          `json:"observations"`
	Attention        []Attention            `json:"attention"`
	JoinRequestCount *Uint                  `json:"join_request_count,omitempty"`
	Effects          []Effect               `json:"effects"`
	Review           *Review                `json:"review,omitempty"`
	Cursor           string                 `json:"cursor"`
	Error            *Error                 `json:"error,omitempty"`
	Files            []FileEntry            `json:"files,omitempty"`
	File             *FileDetail            `json:"file,omitempty"`
}

// FileEntry is one row of the read-only Files view. State is one of the EG2
// labels (captured, waiting_capture, waiting_publish, downloading,
// content_missing, conflict, blocked, deleted) or empty for an implicit
// directory; it describes this device only.
type FileEntry struct {
	Path      string `json:"path"`
	Name      string `json:"name"`
	Directory bool   `json:"directory"`
	Bytes     Uint   `json:"bytes"`
	Modified  string `json:"modified"` // RFC 3339 UTC from the last scan; empty when not on disk here
	State     string `json:"state"`
	Reason    string `json:"reason,omitempty"`
}

// fileStates are the EG2 labels shared by the Files view and `orbit files`.
// Each describes this device only.
var fileStates = map[string][2]string{
	"captured":        {"Saved here", "This device has recorded this version and it is in the folder."},
	"waiting_publish": {"Arriving", "A newer version is stored here and is being written to the folder."},
	"downloading":     {"Downloading", "A newer version is known; its content is still arriving."},
	"content_missing": {"Content missing", "The content of the newest version is unavailable here; check integrity."},
	"conflict":        {"Conflict", "Devices changed this path concurrently. Review the conflict."},
	"blocked":         {"Blocked", "Orbit could not capture or write this entry here. Check Attention."},
	"deleted":         {"Deleted", "The newest version is a deletion. An earlier version can be restored from history."},
}

// FileStateLabel names a FileEntry state for people; unknown or empty is "".
func FileStateLabel(state string) string { return fileStates[state][0] }

// FileStateHelp explains a FileEntry state in one sentence.
func FileStateHelp(state string) string { return fileStates[state][1] }

// FileDetail adds the current versions and what other devices last reported
// about the newest one. Each observation's ObservedAt is the time of that
// report, never the time of this query.
type FileDetail struct {
	Entry        FileEntry        `json:"entry"`
	LastChecked  string           `json:"last_checked"`
	Heads        []VersionSummary `json:"heads"`
	Observations []Observation    `json:"observations"`
}

// Client is an injection seam, not a second engine. Upload is bounded streamed
// staging; its ID/digest/size enter the subsequently reviewed content mutation.
type Client interface {
	Query(context.Context, Query) (Result, error)
	Mutate(context.Context, Mutation) (Result, error)
	Read(context.Context, ReadIntent) (io.ReadCloser, error)
	Upload(context.Context, UploadIntent, io.Reader) (Result, error)
}

type VersionSummary struct {
	Version      VersionID `json:"version"`
	Path         string    `json:"path"`
	Kind         string    `json:"kind"`
	DeviceName   string    `json:"device_name"`
	DisplayTime  string    `json:"display_time"`
	Bytes        Uint      `json:"bytes"`
	Digest       string    `json:"digest"`
	Availability string    `json:"availability"`
}
type ContentReview struct {
	Context           Context         `json:"context"`
	Review            Review          `json:"review"`
	Heads             []VersionID     `json:"heads"`
	Source            *VersionSummary `json:"source,omitempty"`
	WorkingCaptured   bool            `json:"working_captured"`
	ReplacementPaths  []string        `json:"replacement_paths"`
	Destination       string          `json:"destination"`
	DestinationReview *Review         `json:"destination_review,omitempty"`
}

// UploadIntent admits immutable staged bytes without committing a resolution.
type UploadIntent struct {
	OperationID string `json:"operation_id"`
	Session     string `json:"session"`
	Bytes       Uint   `json:"bytes"`
	Digest      string `json:"digest"`
}

// UploadResult describes immutable staged content; it is not a captured version.
type UploadResult struct {
	ID        string `json:"id"`
	Session   string `json:"session"`
	Bytes     Uint   `json:"bytes"`
	Digest    string `json:"digest"`
	ExpiresAt string `json:"expires_at"`
}
