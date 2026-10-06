package control

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/calebhabesh/file-sync/internal/config"
	"github.com/calebhabesh/file-sync/internal/repository"
)

type SupportCategory struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	ItemCount   int    `json:"item_count"`
}

type SupportPreviewResult struct {
	Categories    []SupportCategory `json:"categories"`
	EstimatedSize uint64            `json:"estimated_size"`
	RedactedPaths bool              `json:"redacted_paths"`
}

type SupportExportRequest struct {
	DestinationPath string `json:"destination_path"`
	RedactPaths     bool   `json:"redact_paths"`
}

type SupportExportResult struct {
	ArchivePath    string            `json:"archive_path"`
	Categories     []SupportCategory `json:"categories"`
	TotalFiles     int               `json:"total_files"`
	TotalBytes     uint64            `json:"total_bytes"`
	RedactedPaths  bool              `json:"redacted_paths"`
	RedactionCount int               `json:"redaction_count"`
}

type pathRedactor struct {
	mapping map[string]string
	count   int
}

func newPathRedactor() *pathRedactor {
	return &pathRedactor{
		mapping: make(map[string]string),
	}
}

func (r *pathRedactor) Redact(p string) string {
	if p == "" {
		return ""
	}
	if val, ok := r.mapping[p]; ok {
		return val
	}
	r.count++
	h := sha256.Sum256([]byte(p))
	pseudonym := fmt.Sprintf("path_%s", hex.EncodeToString(h[:4]))
	r.mapping[p] = pseudonym
	return pseudonym
}

func (c *Controller) PreviewSupportExport(ctx context.Context, redactPaths bool) (*SupportPreviewResult, error) {
	docReport, _ := c.Doctor(ctx)
	docCount := 0
	if docReport != nil {
		docCount = len(docReport.Checks)
	}

	folders, _ := c.db.Folders(ctx)
	tasks, _ := c.db.ListDurableTasks(ctx, repository.TaskFilter{Limit: 100})
	events, _ := c.db.ListEvents(ctx, 100)

	cats := []SupportCategory{
		{Name: "doctor", Description: "Diagnostics health checks and remediation", ItemCount: docCount},
		{Name: "system", Description: "OS, architecture, runtime, and schema metadata", ItemCount: 1},
		{Name: "config", Description: "Sanitized agent configuration (secrets redacted)", ItemCount: 1},
		{Name: "membership", Description: "Folder membership revisions and peer entries", ItemCount: len(folders)},
		{Name: "storage", Description: "Storage breakdown by category", ItemCount: 1},
		{Name: "work", Description: "Durable work queue tasks and states", ItemCount: len(tasks)},
		{Name: "events", Description: "Bounded recent structured operation events", ItemCount: len(events)},
	}

	var estimated uint64
	for _, cat := range cats {
		estimated += uint64(cat.ItemCount*128 + 256)
	}

	return &SupportPreviewResult{
		Categories:    cats,
		EstimatedSize: estimated,
		RedactedPaths: redactPaths,
	}, nil
}

func (c *Controller) ExportSupportBundle(ctx context.Context, req SupportExportRequest) (*SupportExportResult, error) {
	if req.DestinationPath == "" {
		ts := time.Now().UTC().Format("20060102-150405")
		req.DestinationPath = fmt.Sprintf("support-bundle-%s.tar.gz", ts)
	}

	redactor := newPathRedactor()

	// 1. Doctor
	docReport, err := c.Doctor(ctx)
	if err != nil {
		return nil, fmt.Errorf("generate doctor report for export: %w", err)
	}
	// Diagnostic prose and remediation may include paths or nested OS errors.
	// Keep structured names/status, withhold free text in a redacted export.
	for i := range docReport.Checks {
		if req.RedactPaths {
			docReport.Checks[i].Message = redactor.Redact(docReport.Checks[i].Message)
			docReport.Checks[i].Remediation = redactor.Redact(docReport.Checks[i].Remediation)
		}
	}

	docBytes, _ := json.MarshalIndent(docReport, "", "  ")

	// 2. System metadata
	sysInfo := map[string]any{
		"os":             runtime.GOOS,
		"arch":           runtime.GOARCH,
		"go_version":     runtime.Version(),
		"schema_version": repository.CurrentSchema,
		"timestamp":      time.Now().UTC().Format(time.RFC3339),
	}
	sysBytes, _ := json.MarshalIndent(sysInfo, "", "  ")

	// 3. Config (sanitized)
	// Whitelist the typed identity metadata; never copy arbitrary config fields.
	cfg, _ := config.Load(c.db.StateDir())
	cfgBytes, _ := json.MarshalIndent(map[string]any{"format_version": cfg.FormatVersion, "device_id": cfg.DeviceID, "created_at": cfg.CreatedAt}, "", "  ")

	// 4. Membership & Folders
	folders, _ := c.db.Folders(ctx)
	type sanitizedFolder struct {
		Folder             string `json:"folder"`
		LocalAuthor        string `json:"local_author"`
		NextCounter        uint64 `json:"next_counter"`
		MembershipRevision uint64 `json:"membership_revision"`
		RootPath           string `json:"root_path,omitempty"`
		Paused             bool   `json:"paused"`
		PauseReason        string `json:"pause_reason,omitempty"`
		ActivePeers        int    `json:"active_peers"`
	}
	var folderEntries []sanitizedFolder
	for _, f := range folders {
		rootP := f.RootPath
		if req.RedactPaths && rootP != "" {
			rootP = redactor.Redact(rootP)
		}
		pl, _ := c.PeerList(ctx, f.Folder)
		folderEntries = append(folderEntries, sanitizedFolder{
			Folder:             hex.EncodeToString(f.Folder[:]),
			LocalAuthor:        hex.EncodeToString(f.LocalAuthor[:]),
			NextCounter:        f.NextCounter,
			MembershipRevision: f.MembershipRevision,
			RootPath:           rootP,
			Paused:             f.Paused,
			PauseReason:        supportCode(f.PauseReason),
			ActivePeers:        len(pl.Active),
		})
	}
	membershipBytes, _ := json.MarshalIndent(folderEntries, "", "  ")

	// 5. Storage
	storage, _ := c.StorageUsage(ctx)
	if req.RedactPaths {
		storage.Usage.StateFilesystem.Path = redactor.Redact(storage.Usage.StateFilesystem.Path)
		for i := range storage.Usage.Folders {
			if storage.Usage.Folders[i].RootPath != "" {
				storage.Usage.Folders[i].RootPath = redactor.Redact(storage.Usage.Folders[i].RootPath)
			}
			if storage.Usage.Folders[i].Filesystem.Path != "" {
				storage.Usage.Folders[i].Filesystem.Path = redactor.Redact(storage.Usage.Folders[i].Filesystem.Path)
			}
		}
	}
	storageBytes, _ := json.MarshalIndent(storage, "", "  ")

	// 6. Work Tasks
	tasks, _ := c.db.ListDurableTasks(ctx, repository.TaskFilter{Limit: 100})
	if req.RedactPaths {
		for i := range tasks {
			if tasks[i].TargetPath != "" {
				tasks[i].TargetPath = redactor.Redact(tasks[i].TargetPath)
			}
		}
	}
	for i := range tasks {
		tasks[i].LastError = ""
		tasks[i].ErrorCode = supportCode(tasks[i].ErrorCode)
	}
	tasksBytes, _ := json.MarshalIndent(tasks, "", "  ")

	// 7. Events
	events, _ := c.db.ListEvents(ctx, 100)
	for i := range events {
		events[i].ErrorCode = supportCode(events[i].ErrorCode)
		events[i].Phase = supportCode(events[i].Phase)
	}
	eventsBytes, _ := json.MarshalIndent(events, "", "  ")

	// 8. Metrics
	metrics, _ := c.Metrics(ctx)
	if metrics != nil {
		clean := map[string]uint64{}
		for code, count := range metrics.RetryCauses {
			clean[supportCode(code)] += count
		}
		metrics.RetryCauses = clean
	}
	metricsBytes, _ := json.MarshalIndent(metrics, "", "  ")

	// 9. Manifest
	bundleManifest := map[string]any{
		"bundle_version": 1,
		"schema_version": repository.CurrentSchema,
		"generated_at":   time.Now().UTC().Format(time.RFC3339),
		"redacted_paths": req.RedactPaths,
		"files": []string{
			"manifest.json", "doctor.json", "system.json", "config.json",
			"membership.json", "storage.json", "work.json", "events.json", "metrics.json",
		},
	}
	manifestBytes, _ := json.MarshalIndent(bundleManifest, "", "  ")

	filesToArchive := map[string][]byte{
		"manifest.json":   manifestBytes,
		"doctor.json":     docBytes,
		"system.json":     sysBytes,
		"config.json":     cfgBytes,
		"membership.json": membershipBytes,
		"storage.json":    storageBytes,
		"work.json":       tasksBytes,
		"events.json":     eventsBytes,
		"metrics.json":    metricsBytes,
	}

	// Create output tar.gz
	f, err := os.OpenFile(req.DestinationPath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create support archive %s: %w", req.DestinationPath, err)
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	var totalBytes uint64
	for name, data := range filesToArchive {
		hdr := &tar.Header{
			Name:    name,
			Mode:    0o600,
			Size:    int64(len(data)),
			ModTime: time.Now().UTC(),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return nil, fmt.Errorf("write tar header %s: %w", name, err)
		}
		if _, err := tw.Write(data); err != nil {
			return nil, fmt.Errorf("write tar data %s: %w", name, err)
		}
		totalBytes += uint64(len(data))
	}

	cats := []SupportCategory{
		{Name: "doctor", Description: "Diagnostics health checks and remediation", ItemCount: len(docReport.Checks)},
		{Name: "system", Description: "OS, architecture, runtime, and schema metadata", ItemCount: 1},
		{Name: "config", Description: "Sanitized agent configuration (secrets redacted)", ItemCount: 1},
		{Name: "membership", Description: "Folder membership revisions and peer entries", ItemCount: len(folderEntries)},
		{Name: "storage", Description: "Storage breakdown by category", ItemCount: 1},
		{Name: "work", Description: "Durable work queue tasks and states", ItemCount: len(tasks)},
		{Name: "events", Description: "Bounded recent structured operation events", ItemCount: len(events)},
	}

	return &SupportExportResult{
		ArchivePath:    req.DestinationPath,
		Categories:     cats,
		TotalFiles:     len(filesToArchive),
		TotalBytes:     totalBytes,
		RedactedPaths:  req.RedactPaths,
		RedactionCount: redactor.count,
	}, nil
}

// Only bounded structured reason codes belong in export. Arbitrary error text
// could contain invitation tokens, ICE passwords or private paths.
func supportCode(s string) string {
	if s == "" {
		return ""
	}
	if len(s) > 64 {
		return "[REDACTED]"
	}
	for _, c := range s {
		if !(c >= 'A' && c <= 'Z') && c != '_' && !(c >= '0' && c <= '9') {
			return "[REDACTED]"
		}
	}
	return s
}
