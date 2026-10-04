package control

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tc "github.com/calebhabesh/file-sync/internal/control/terminalcontract"
	"github.com/calebhabesh/file-sync/internal/repository"
)

func (c *Controller) terminalFolders(ctx context.Context, q tc.Query) (tc.Result, error) {
	r := terminalResult()
	if q.Kind == "folders" && (q.Limit > 0 || q.Cursor != "") {
		var err error
		r.Items, r.Cursor, err = c.db.NamedPage(ctx, q, "")
		if err != nil {
			return r, err
		}
		if len(r.Items) == 0 {
			r.State = "empty"
		} else {
			r.State = "success"
		}
		return r, nil
	}
	items, err := c.db.NamedFolders(ctx)
	if err != nil {
		return r, err
	}
	if q.Folder != "" {
		filtered := make([]tc.NamedItem, 0, len(items))
		for _, it := range items {
			if it.ID == q.Folder {
				filtered = append(filtered, it)
			}
		}
		items = filtered
	}
	if q.Name != "" {
		filtered := make([]tc.NamedItem, 0, len(items))
		for _, it := range items {
			if it.Name == q.Name || filepath.Base(it.Root) == q.Name {
				filtered = append(filtered, it)
			}
		}
		items = filtered
	}
	r.Items = items
	if len(items) == 0 {
		r.State = "empty"
	} else {
		r.State = "success"
	}
	return r, nil
}

func (c *Controller) terminalDevices(ctx context.Context, q tc.Query) (tc.Result, error) {
	r := terminalResult()
	if q.Kind == "devices" && (q.Limit > 0 || q.Cursor != "") {
		local := ""
		if c.options.LocalDevice != ([32]byte{}) {
			local = hex.EncodeToString(c.options.LocalDevice[:])
		}
		var err error
		r.Items, r.Cursor, err = c.db.NamedPage(ctx, q, local)
		if err != nil {
			return r, err
		}
		if len(r.Items) == 0 {
			r.State = "empty"
		} else {
			r.State = "success"
		}
		return r, nil
	}
	items, err := c.db.NamedDevices(ctx)
	if err != nil {
		return r, err
	}
	if c.options.LocalDevice != ([32]byte{}) {
		localHex := hex.EncodeToString(c.options.LocalDevice[:])
		found := false
		for _, it := range items {
			if it.ID == localHex {
				found = true
				break
			}
		}
		if !found {
			label, _ := c.db.GetDeviceDisplayName(ctx, c.options.LocalDevice)
			if label == "" {
				label = "This device"
			}
			items = append([]tc.NamedItem{{
				ID:   localHex,
				Name: label,
			}}, items...)
		}
	}
	if q.ID != "" {
		filtered := make([]tc.NamedItem, 0, len(items))
		for _, it := range items {
			if it.ID == q.ID {
				filtered = append(filtered, it)
			}
		}
		items = filtered
	}
	if q.Name != "" {
		filtered := make([]tc.NamedItem, 0, len(items))
		for _, it := range items {
			if it.Name == q.Name {
				filtered = append(filtered, it)
			}
		}
		items = filtered
	}
	r.Items = items
	if len(items) == 0 {
		r.State = "empty"
	} else {
		r.State = "success"
	}
	return r, nil
}

func (c *Controller) terminalContext(ctx context.Context, q tc.Query) (tc.Result, error) {
	r := terminalResult()
	folders, err := c.db.RegisteredFolders(ctx)
	if err != nil {
		return r, err
	}
	named, err := c.db.NamedFolders(ctx)
	if err != nil {
		return r, err
	}
	if len(folders) == 0 {
		r.State = "root_unavailable"
		r.Error = &tc.Error{
			Code:      "ROOT_UNAVAILABLE",
			Message:   "no folders are currently registered",
			Retryable: false,
			Action:    "create or join a folder with orbit setup",
		}
		return r, nil
	}

	var matchedFolder *repository.RootRegistration
	var matchedName string
	var cleanCwd string
	if q.Cwd != "" {
		cleanCwd = filepath.Clean(q.Cwd)
	}

	// Step 1: Explicit folder ID if provided
	if q.Folder != "" {
		var matchedNamed *tc.NamedItem
		for i := range folders {
			fHex := hex.EncodeToString(folders[i].Folder[:])
			if fHex == q.Folder {
				matchedFolder = &folders[i]
				matchedName = named[i].Name
				matchedNamed = &named[i]
				break
			}
		}
		if matchedFolder == nil {
			r.State = "failed"
			r.Error = &tc.Error{
				Code:      "FOLDER_NOT_FOUND",
				Message:   fmt.Sprintf("folder %s not found", q.Folder),
				Retryable: false,
				Action:    "check available folders with orbit folders",
			}
			r.Items = named
			return r, nil
		}
		if q.Name != "" && matchedName != q.Name && filepath.Base(matchedFolder.Path) != q.Name {
			r.State = "ambiguous"
			r.Error = &tc.Error{
				Code:      "AMBIGUOUS_CONTEXT",
				Message:   fmt.Sprintf("folder %s name %q does not match %q", q.Folder, matchedName, q.Name),
				Retryable: false,
				Action:    "specify either folder ID or matching folder name",
			}
			r.Items = []tc.NamedItem{*matchedNamed}
			return r, nil
		}
	} else if q.Name != "" {
		// Step 2: Name specified without Folder ID
		var candidates []tc.NamedItem
		var regCandidates []*repository.RootRegistration
		for i := range folders {
			n := named[i].Name
			b := filepath.Base(folders[i].Path)
			if n == q.Name || b == q.Name {
				candidates = append(candidates, named[i])
				regCandidates = append(regCandidates, &folders[i])
			}
		}
		if len(candidates) == 0 {
			r.State = "failed"
			r.Error = &tc.Error{
				Code:      "FOLDER_NOT_FOUND",
				Message:   fmt.Sprintf("no folder matches name %q", q.Name),
				Retryable: false,
				Action:    "check available folders with orbit folders",
			}
			r.Items = named
			return r, nil
		}
		if len(candidates) > 1 {
			r.State = "ambiguous"
			r.Error = &tc.Error{
				Code:      "AMBIGUOUS_CONTEXT",
				Message:   fmt.Sprintf("multiple folders match name %q", q.Name),
				Retryable: false,
				Action:    "specify folder by ID",
			}
			r.Items = candidates
			return r, nil
		}
		matchedFolder = regCandidates[0]
		matchedName = candidates[0].Name
	} else {
		// Step 3: Infer from Cwd
		if cleanCwd == "" || !filepath.IsAbs(cleanCwd) {
			r.State = "ambiguous"
			r.Error = &tc.Error{
				Code:      "AMBIGUOUS_CONTEXT",
				Message:   "current directory is not inside a synced folder and no folder was specified",
				Retryable: false,
				Action:    "specify --folder <name> or navigate to a synced folder",
			}
			r.Items = named
			return r, nil
		}
		var bestMatch *repository.RootRegistration
		var bestName string
		longestRoot := -1
		for i := range folders {
			root := filepath.Clean(folders[i].Path)
			if cleanCwd == root || strings.HasPrefix(cleanCwd, root+string(filepath.Separator)) {
				if len(root) > longestRoot {
					longestRoot = len(root)
					bestMatch = &folders[i]
					bestName = named[i].Name
				}
			}
		}
		if bestMatch == nil {
			r.State = "ambiguous"
			r.Error = &tc.Error{
				Code:      "AMBIGUOUS_CONTEXT",
				Message:   fmt.Sprintf("current directory %s is not inside any synced folder", cleanCwd),
				Retryable: false,
				Action:    "specify --folder <name> or navigate to a synced folder",
			}
			r.Items = named
			return r, nil
		}
		matchedFolder = bestMatch
		matchedName = bestName
	}

	// Step 4: Verify root accessibility and health
	stat, err := os.Stat(matchedFolder.Path)
	if err != nil || !stat.IsDir() {
		r.State = "root_unavailable"
		r.Error = &tc.Error{
			Code:      "ROOT_UNAVAILABLE",
			Message:   fmt.Sprintf("registered root %s is unavailable", matchedFolder.Path),
			Retryable: true,
			Action:    "ensure the root directory exists or relocate folder",
		}
		return r, nil
	}

	if c.ws != nil {
		if err := c.ws.Revalidate(ctx, matchedFolder.Folder); err != nil {
			r.State = "stale"
			r.Error = &tc.Error{
				Code:      "STALE_ROOT",
				Message:   fmt.Sprintf("root registration invalid or relocated: %v", err),
				Retryable: false,
				Action:    "relocate folder with orbit folders relocate",
			}
			return r, nil
		}
	}

	// Step 5: Check Path if provided
	if q.Path != "" {
		if strings.HasPrefix(q.Path, ".orbit-") || strings.Contains(q.Path, "/.orbit-") || strings.HasPrefix(q.Path, ".filesync") || strings.Contains(q.Path, "/.filesync") {
			r.State = "failed"
			r.Error = &tc.Error{
				Code:      "INVALID_PATH",
				Message:   "path accesses reserved Orbit directory",
				Retryable: false,
				Action:    "select a normal workspace file",
			}
			return r, nil
		}
		fullPath := filepath.Join(matchedFolder.Path, filepath.FromSlash(q.Path))
		if lstat, err := os.Lstat(fullPath); err == nil {
			if lstat.Mode()&os.ModeSymlink != 0 {
				r.State = "failed"
				r.Error = &tc.Error{
					Code:      "INVALID_PATH",
					Message:   "symlinked path is forbidden",
					Retryable: false,
					Action:    "use a regular path within the synced folder",
				}
				return r, nil
			}
		}
	}

	// Step 6: Generation binds registration details and folder name
	genObj := struct {
		Folder string `json:"folder"`
		Name   string `json:"name"`
		Root   string `json:"root"`
		Device uint64 `json:"device"`
		Inode  uint64 `json:"inode"`
		RegID  string `json:"reg_id"`
	}{
		Folder: hex.EncodeToString(matchedFolder.Folder[:]),
		Name:   matchedName,
		Root:   matchedFolder.Path,
		Device: matchedFolder.Device,
		Inode:  matchedFolder.Inode,
		RegID:  hex.EncodeToString(matchedFolder.RegistrationID[:]),
	}
	gen := generation(genObj)

	resolvedRelPath := q.Path
	if resolvedRelPath == "" && cleanCwd != "" {
		cleanRoot := filepath.Clean(matchedFolder.Path)
		rel, err := filepath.Rel(cleanRoot, cleanCwd)
		if err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
			resolvedRelPath = filepath.ToSlash(rel)
		}
	}

	r.Context = &tc.Context{
		Folder:     hex.EncodeToString(matchedFolder.Folder[:]),
		FolderName: matchedName,
		Root:       matchedFolder.Path,
		Path:       resolvedRelPath,
		Generation: gen,
	}
	r.State = "success"
	return r, nil
}
