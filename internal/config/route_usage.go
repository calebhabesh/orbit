package config

import (
	"encoding/json"
	"errors"
	"os"

	"github.com/calebhabesh/orbit/internal/network"
	"github.com/calebhabesh/orbit/internal/protocol"
	"github.com/calebhabesh/orbit/internal/state"
)

// RouteUsageFile holds this device's route accounting (RG2): owner-only,
// bucket totals per peer device, purpose and route class, no addresses or
// file names, 90-day retention.
const RouteUsageFile = "route-usage.json"

const maxRouteUsageBytes = 8 << 20

type routeUsageDoc struct {
	Version string                     `json:"version"`
	Records []network.RouteUsageRecord `json:"records"`
}

// LoadRouteUsage returns the durable record; a missing file is empty.
func LoadRouteUsage(dir string) ([]network.RouteUsageRecord, error) {
	b, err := state.ReadPrivate(dir, RouteUsageFile, maxRouteUsageBytes)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var doc routeUsageDoc
	if err = protocol.DecodeStrict(b, &doc); err != nil {
		return nil, err
	}
	if doc.Version != "1" {
		return nil, errors.New("ROUTE_USAGE_INVALID")
	}
	return doc.Records, nil
}

// SaveRouteUsage replaces the durable record atomically.
func SaveRouteUsage(dir string, records []network.RouteUsageRecord) error {
	if records == nil {
		records = []network.RouteUsageRecord{}
	}
	b, err := json.Marshal(routeUsageDoc{Version: "1", Records: records})
	if err != nil {
		return err
	}
	return WritePrivate(dir, RouteUsageFile, b)
}
