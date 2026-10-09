package replication

import (
	"context"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"

	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/repository"
)

// Shared names (2.2.0): each Orbit's name and its members' device names are
// last-writer-wins records exchanged after a successful hello. They never
// enter membership digests or file history, so a rename cannot pause sync.
// Peers without the endpoint answer 404 and keep their local names.

const namesPath = "/peer/v1/names"

type NameWire struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Clock  string `json:"clock"`
	Author string `json:"author"`
}

type NamesRequest struct {
	ProtocolVersion  string     `json:"protocol_version"`
	DeviceID         string     `json:"device_id"`
	FolderID         string     `json:"folder_id"`
	Revision         string     `json:"membership_revision"`
	MembershipDigest string     `json:"membership_digest"`
	Folder           *NameWire  `json:"folder,omitempty"`
	Devices          []NameWire `json:"devices"`
}

type NamesResponse struct {
	ProtocolVersion string     `json:"protocol_version"`
	Folder          *NameWire  `json:"folder,omitempty"`
	Devices         []NameWire `json:"devices"`
}

// NamesExchanger is implemented by peer clients that can exchange names.
type NamesExchanger interface {
	Names(context.Context, NamesRequest) (NamesResponse, error)
}

func (client *Client) Names(ctx context.Context, request NamesRequest) (NamesResponse, error) {
	var response NamesResponse
	return response, client.postJSON(ctx, namesPath, request, &response)
}

func toWire(r repository.NameRecord) NameWire {
	return NameWire{ID: hex.EncodeToString(r.ID[:]), Name: r.Name, Clock: strconv.FormatUint(r.Clock, 10), Author: hex.EncodeToString(r.Author[:])}
}

func fromWire(w NameWire) (repository.NameRecord, error) {
	id, err := parseID(w.ID)
	if err != nil {
		return repository.NameRecord{}, err
	}
	author, err := parseID(w.Author)
	if err != nil {
		return repository.NameRecord{}, err
	}
	clock, err := parseDecimal(w.Clock, false)
	if err != nil {
		return repository.NameRecord{}, err
	}
	if err := repository.ValidName(w.Name); err != nil {
		return repository.NameRecord{}, err
	}
	return repository.NameRecord{ID: id, Name: w.Name, Clock: clock, Author: author}, nil
}

// localNames collects the shared records for a folder and its active members.
func localNames(ctx context.Context, repo *repository.DB, folder history.ID) (*NameWire, []NameWire, map[history.ID]bool, error) {
	membership, _, err := repo.GetMembership(ctx, folder)
	if err != nil {
		return nil, nil, nil, err
	}
	members := make(map[history.ID]bool, len(membership.Active))
	var devices []NameWire
	for _, m := range membership.Active {
		members[m.Device] = true
		r, err := repo.DeviceNameRecord(ctx, m.Device)
		if err != nil {
			return nil, nil, nil, err
		}
		if r.Clock > 0 {
			devices = append(devices, toWire(r))
		}
	}
	var own *NameWire
	if r, err := repo.FolderNameRecord(ctx, folder); err != nil {
		return nil, nil, nil, err
	} else if r.Clock > 0 {
		w := toWire(r)
		own = &w
	}
	if devices == nil {
		devices = []NameWire{}
	}
	return own, devices, members, nil
}

// mergeNames installs newer records for this folder and its active members.
// Malformed records fail the exchange; records about or by devices that are
// not active members here (for example retired since) are skipped.
func mergeNames(ctx context.Context, repo *repository.DB, folder history.ID, members map[history.ID]bool, own *NameWire, devices []NameWire) error {
	if len(devices) > len(members)+history.MaxIdentities {
		return errors.New("too many device names")
	}
	if own != nil {
		r, err := fromWire(*own)
		if err != nil || r.ID != folder {
			return errors.New("invalid folder name record")
		}
		if members[r.Author] {
			if _, err := repo.MergeFolderName(ctx, r); err != nil {
				return err
			}
		}
	}
	for _, w := range devices {
		r, err := fromWire(w)
		if err != nil {
			return errors.New("invalid device name record")
		}
		if !members[r.ID] || !members[r.Author] {
			continue
		}
		if _, err := repo.MergeDeviceName(ctx, r); err != nil {
			return err
		}
	}
	return nil
}

func (server *Server) handleNames(writer http.ResponseWriter, request *http.Request) {
	var body NamesRequest
	if !readRequest(writer, request, &body) {
		return
	}
	if body.ProtocolVersion != ProtocolVersion {
		writeWireError(writer, http.StatusUpgradeRequired, "INCOMPATIBLE_VERSION", "protocol version is not supported", false, "use protocol version 1")
		return
	}
	_, folder, err := server.authorize(request, body.DeviceID, body.FolderID, body.Revision, body.MembershipDigest)
	if err != nil {
		server.writeAuthorizationError(writer, err)
		return
	}
	ctx := request.Context()
	_, _, members, err := localNames(ctx, server.repo, folder)
	if err != nil {
		writeWireError(writer, http.StatusInternalServerError, "IO_ERROR", "cannot read names", true, "retry later")
		return
	}
	if err := mergeNames(ctx, server.repo, folder, members, body.Folder, body.Devices); err != nil {
		writeWireError(writer, http.StatusBadRequest, "INVALID_REQUEST", "name records are invalid", false, "send printable names for this folder's members")
		return
	}
	own, devices, _, err := localNames(ctx, server.repo, folder)
	if err != nil {
		writeWireError(writer, http.StatusInternalServerError, "IO_ERROR", "cannot read names", true, "retry later")
		return
	}
	writeJSON(writer, http.StatusOK, NamesResponse{ProtocolVersion: ProtocolVersion, Folder: own, Devices: devices})
}

// exchangeNames sends this device's records and installs the peer's newer
// ones. A peer without the endpoint, or any failure, leaves names as they are.
func (syncer *Syncer) exchangeNames(ctx context.Context) {
	nx, ok := syncer.client.(NamesExchanger)
	if !ok {
		return
	}
	own, devices, members, err := localNames(ctx, syncer.repo, syncer.folder)
	if err != nil {
		return
	}
	device, folder, revision, digest := syncer.common()
	response, err := nx.Names(ctx, NamesRequest{ProtocolVersion: ProtocolVersion, DeviceID: device, FolderID: folder, Revision: revision, MembershipDigest: digest, Folder: own, Devices: devices})
	if err != nil || response.ProtocolVersion != ProtocolVersion {
		return
	}
	_ = mergeNames(ctx, syncer.repo, syncer.folder, members, response.Folder, response.Devices)
}
