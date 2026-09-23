// Package model is a deliberately simple test oracle. It uses explicit parent
// reachability and must not import the production history implementation.
package model

import (
	"errors"
	"fmt"
	"sort"
)

var ErrMissingParent = errors.New("oracle missing parent")

type Event struct {
	ID, Author, Path string
	Parents          []string
	Kind             string
	Content          string
}

type DAG struct {
	events    map[string]Event
	available map[string]bool
}

// Admission is the oracle's intentionally string-keyed membership view. It is
// separate from DAG reachability so rejected ancestry cannot become accepted
// merely because an envelope is otherwise well formed.
type Admission struct {
	MembershipMatches bool
	Active            map[string]bool
	Retired           map[string]map[uint64]string
}

func (a Admission) Allows(author string, counter uint64, digest string, parentsAdmitted bool) bool {
	if !a.MembershipMatches || !parentsAdmitted {
		return false
	}
	if a.Active[author] {
		return true
	}
	known, retired := a.Retired[author]
	return retired && known[counter] == digest && digest != ""
}

func New() *DAG { return &DAG{events: map[string]Event{}, available: map[string]bool{}} }
func (d *DAG) Accept(event Event, contentAvailable bool) error {
	if old, ok := d.events[event.ID]; ok {
		if fmt.Sprint(old) == fmt.Sprint(event) {
			if contentAvailable {
				d.available[event.ID] = true
			}
			return nil
		}
		return errors.New("oracle duplicate identity mismatch")
	}
	for _, p := range event.Parents {
		parent, ok := d.events[p]
		if !ok {
			return fmt.Errorf("%w: %s", ErrMissingParent, p)
		}
		if parent.Path != event.Path {
			return errors.New("oracle parent path mismatch")
		}
	}
	event.Parents = append([]string(nil), event.Parents...)
	d.events[event.ID] = event
	d.available[event.ID] = contentAvailable
	return nil
}
func (d *DAG) reaches(from, to string) bool {
	seen := map[string]bool{}
	var visit func(string) bool
	visit = func(id string) bool {
		if id == to {
			return true
		}
		if seen[id] {
			return false
		}
		seen[id] = true
		for _, p := range d.events[id].Parents {
			if visit(p) {
				return true
			}
		}
		return false
	}
	return visit(from)
}
func (d *DAG) Heads(path string) []string {
	var ids []string
	for id, e := range d.events {
		if e.Path == path {
			ids = append(ids, id)
		}
	}
	var heads []string
	for _, id := range ids {
		dominated := false
		for _, other := range ids {
			if other != id && d.reaches(other, id) {
				dominated = true
				break
			}
		}
		if !dominated {
			heads = append(heads, id)
		}
	}
	sort.Strings(heads)
	return heads
}
func (d *DAG) Available(id string) bool { return d.available[id] }
