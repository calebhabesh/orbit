package control

import (
	"errors"
	"fmt"

	"github.com/calebhabesh/orbit/internal/history"
)

var (
	ErrUnauthorized         = errors.New("unauthorized")
	ErrMembershipMismatch   = errors.New("membership revision mismatch")
	ErrIncompatibleVersion  = errors.New("incompatible version")
	ErrInvalidManifest      = errors.New("invalid manifest")
	ErrInvalidPath          = errors.New("invalid path")
	ErrStaleView            = errors.New("conflict heads changed before resolution")
	ErrRootUnavailable      = errors.New("workspace root is unavailable")
	ErrStructuralConflict   = errors.New("structural conflict")
	ErrContentPending       = errors.New("version content is still pending transfer")
	ErrContentExpired       = errors.New("version content payload has expired under retention policy")
	ErrContentUnavailable   = errors.New("version content is unavailable locally")
	ErrDiskBudget           = errors.New("disk budget or storage reserve exhausted")
	ErrIOError              = errors.New("filesystem I/O error")
	ErrUnstableFile         = errors.New("file is unstable")
	ErrRetryExhausted       = errors.New("task retry limit exhausted")
	ErrDestinationCollision = errors.New("destination path already exists")
	ErrIdempotencyConflict  = errors.New("idempotency key was previously used with different parameters")
	ErrExpiredReplay        = errors.New("idempotency record has expired")
	ErrInvalidRequest       = errors.New("invalid control request")
	ErrMembershipFork       = errors.New("membership fork detected: concurrent conflicting revisions")
	ErrRetiredMemberRevival = errors.New("cannot revive retired member")
	ErrInvalidSignature     = errors.New("joining signature verification failed")
	ErrRateLimitExceeded    = errors.New("rate limit exceeded")
	ErrPayloadTooLarge      = errors.New("request payload exceeds bounded limit")
)

type ControlError struct {
	Code      string              `json:"code"`
	Message   string              `json:"message"`
	Retryable bool                `json:"retryable"`
	Action    string              `json:"action"`
	Heads     []history.VersionID `json:"heads,omitempty"`
	HeadToken *history.Digest     `json:"head_token,omitempty"`
	Err       error               `json:"-"`
}

func (e *ControlError) Error() string {
	if e.HeadToken != nil {
		return fmt.Sprintf("%s: %s (action: %s, heads: %d, token: %x)", e.Code, e.Message, e.Action, len(e.Heads), *e.HeadToken)
	}
	return fmt.Sprintf("%s: %s (action: %s)", e.Code, e.Message, e.Action)
}

func (e *ControlError) Unwrap() error {
	return e.Err
}

func UnauthorizedError(message string) *ControlError {
	if message == "" {
		message = "missing, invalid, or expired authentication credential"
	}
	return &ControlError{
		Code:      "UNAUTHORIZED",
		Message:   message,
		Retryable: false,
		Action:    "provide valid local credentials or complete browser authentication",
		Err:       ErrUnauthorized,
	}
}

func MembershipMismatchError(message string) *ControlError {
	if message == "" {
		message = "folder membership revision differs or is unapproved"
	}
	return &ControlError{
		Code:      "MEMBERSHIP_MISMATCH",
		Message:   message,
		Retryable: false,
		Action:    "exchange and approve latest membership revision before syncing",
		Err:       ErrMembershipMismatch,
	}
}

func MembershipForkError(message string) *ControlError {
	if message == "" {
		message = "membership fork detected: concurrent conflicting revisions"
	}
	return &ControlError{
		Code:      "MEMBERSHIP_FORK",
		Message:   message,
		Retryable: false,
		Action:    "perform explicit owner reconciliation across forked revisions (Invariant I24)",
		Err:       ErrMembershipFork,
	}
}

func RetiredMemberRevivalError(message string) *ControlError {
	if message == "" {
		message = "cannot revive or readmit retired device identity"
	}
	return &ControlError{
		Code:      "RETIRED_MEMBER_REVIVAL",
		Message:   message,
		Retryable: false,
		Action:    "generate a fresh device identity to enroll a replaced or wiped device (Invariant I24)",
		Err:       ErrRetiredMemberRevival,
	}
}

func InvalidSignatureError(message string) *ControlError {
	if message == "" {
		message = "joining signature verification failed"
	}
	return &ControlError{
		Code:      "INVALID_SIGNATURE",
		Message:   message,
		Retryable: false,
		Action:    "verify private key possession and signature challenge nonce",
		Err:       ErrInvalidSignature,
	}
}

func RateLimitError(message string) *ControlError {
	if message == "" {
		message = "rate limit exceeded for enrollment requests"
	}
	return &ControlError{
		Code:      "RATE_LIMITED",
		Message:   message,
		Retryable: true,
		Action:    "wait before retrying enrollment request",
		Err:       ErrRateLimitExceeded,
	}
}

func PayloadTooLargeError(message string) *ControlError {
	if message == "" {
		message = "request payload exceeds bounded limit (16 KiB)"
	}
	return &ControlError{
		Code:      "PAYLOAD_TOO_LARGE",
		Message:   message,
		Retryable: false,
		Action:    "keep request payload within 16 KiB limit (Invariant I23)",
		Err:       ErrPayloadTooLarge,
	}
}

func IncompatibleVersionError(message string) *ControlError {
	if message == "" {
		message = "protocol or schema version is incompatible with peer or database"
	}
	return &ControlError{
		Code:      "INCOMPATIBLE_VERSION",
		Message:   message,
		Retryable: false,
		Action:    "upgrade orbit agent to a compatible protocol version",
		Err:       ErrIncompatibleVersion,
	}
}

func InvalidManifestError(message string) *ControlError {
	if message == "" {
		message = "manifest structure or chunk list failed verification"
	}
	return &ControlError{
		Code:      "INVALID_MANIFEST",
		Message:   message,
		Retryable: false,
		Action:    "repair corrupted manifest or re-request from authorized peer",
		Err:       ErrInvalidManifest,
	}
}

func InvalidPathError(path string, reason string) *ControlError {
	msg := fmt.Sprintf("invalid path %q", path)
	if reason != "" {
		msg = fmt.Sprintf("invalid path %q: %s", path, reason)
	}
	return &ControlError{
		Code:      "INVALID_PATH",
		Message:   msg,
		Retryable: false,
		Action:    "rename file to satisfy path segment and length constraints",
		Err:       ErrInvalidPath,
	}
}

func StaleViewError(heads []history.VersionID, token history.Digest) *ControlError {
	return &ControlError{
		Code:      "STALE_VIEW",
		Message:   "conflict heads changed before resolution",
		Retryable: false,
		Action:    "review updated conflict heads before resubmitting",
		Heads:     heads,
		HeadToken: &token,
		Err:       ErrStaleView,
	}
}

func RootUnavailableError(path string, message string) *ControlError {
	msg := fmt.Sprintf("workspace root %q is unavailable", path)
	if message != "" {
		msg = fmt.Sprintf("workspace root %q is unavailable: %s", path, message)
	}
	return &ControlError{
		Code:      "ROOT_UNAVAILABLE",
		Message:   msg,
		Retryable: true,
		Action:    "remount or restore access to the workspace root directory",
		Err:       ErrRootUnavailable,
	}
}

func StructuralConflictError(path string, message string) *ControlError {
	msg := fmt.Sprintf("structural conflict at %q", path)
	if message != "" {
		msg = fmt.Sprintf("structural conflict at %q: %s", path, message)
	}
	return &ControlError{
		Code:      "STRUCTURAL_CONFLICT",
		Message:   msg,
		Retryable: false,
		Action:    "resolve file/directory collision or parent directory conflict",
		Err:       ErrStructuralConflict,
	}
}

func ContentPendingError(message string) *ControlError {
	if message == "" {
		message = "version content is still pending transfer"
	}
	return &ControlError{
		Code:      "CONTENT_PENDING",
		Message:   message,
		Retryable: true,
		Action:    "wait for content transfer to complete",
		Err:       ErrContentPending,
	}
}

func ContentExpiredError(message string) *ControlError {
	if message == "" {
		message = "version content payload has expired under retention policy"
	}
	return &ControlError{
		Code:      "CONTENT_EXPIRED",
		Message:   message,
		Retryable: false,
		Action:    "historical payload cannot be restored; inspect other retained versions",
		Err:       ErrContentExpired,
	}
}

func ContentUnavailableError(message string) *ControlError {
	if message == "" {
		message = "version content is unavailable locally and on reachable peers"
	}
	return &ControlError{
		Code:      "CONTENT_UNAVAILABLE",
		Message:   message,
		Retryable: true,
		Action:    "sync with a peer that holds the content or check repair",
		Err:       ErrContentUnavailable,
	}
}

func DiskBudgetError(message string) *ControlError {
	if message == "" {
		message = "storage allocation or reserve disk capacity exhausted"
	}
	return &ControlError{
		Code:      "DISK_BUDGET",
		Message:   message,
		Retryable: false,
		Action:    "free disk space or adjust storage allocation limits",
		Err:       ErrDiskBudget,
	}
}

func IOError(message string) *ControlError {
	if message == "" {
		message = "underlying filesystem or storage I/O operation failed"
	}
	return &ControlError{
		Code:      "IO_ERROR",
		Message:   message,
		Retryable: true,
		Action:    "check filesystem permissions and hardware health",
		Err:       ErrIOError,
	}
}

func UnstableFileError(path string, message string) *ControlError {
	msg := fmt.Sprintf("file %q is unstable", path)
	if message != "" {
		msg = fmt.Sprintf("file %q is unstable: %s", path, message)
	}
	return &ControlError{
		Code:      "UNSTABLE_FILE",
		Message:   msg,
		Retryable: true,
		Action:    "wait for editor or file modification to settle before scanning",
		Err:       ErrUnstableFile,
	}
}

func RetryExhaustedError(taskID string, message string) *ControlError {
	msg := fmt.Sprintf("task %s exceeded retry limit", taskID)
	if message != "" {
		msg = fmt.Sprintf("task %s exceeded retry limit: %s", taskID, message)
	}
	return &ControlError{
		Code:      "RETRY_EXHAUSTED",
		Message:   msg,
		Retryable: false,
		Action:    "inspect persistent failure cause and manually retry task",
		Err:       ErrRetryExhausted,
	}
}

func DestinationCollisionError(path string) *ControlError {
	return &ControlError{
		Code:      "DESTINATION_COLLISION",
		Message:   fmt.Sprintf("destination path %q already exists", path),
		Retryable: false,
		Action:    "specify a different destination path",
		Err:       ErrDestinationCollision,
	}
}

func IdempotencyConflictError(key string) *ControlError {
	return &ControlError{
		Code:      "IDEMPOTENCY_CONFLICT",
		Message:   fmt.Sprintf("idempotency key %q was previously used with different parameters", key),
		Retryable: false,
		Action:    "use a new idempotency key for distinct operations",
		Err:       ErrIdempotencyConflict,
	}
}

func ExpiredReplayError(key string) *ControlError {
	return &ControlError{
		Code:      "EXPIRED_REPLAY",
		Message:   fmt.Sprintf("idempotency record for %q has expired", key),
		Retryable: false,
		Action:    "request a fresh reviewed operation",
		Err:       ErrExpiredReplay,
	}
}
