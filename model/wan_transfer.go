package model

// WANTransfer is an independent durable-boundary oracle. Route state has no
// authority over approved identity, immutable version or receipt eligibility.
// Symbolic chunks are identified separately from production hashes/SQL rows.
type WANTransfer struct {
	Version, Author, Pin     string
	Required                 map[string]bool
	Verified                 map[string]bool
	Metadata, Ready, Receipt bool
	Route                    string
}

func NewWANTransfer(version, author, pin string, chunks []string) *WANTransfer {
	m := &WANTransfer{Version: version, Author: author, Pin: pin, Required: map[string]bool{}, Verified: map[string]bool{}}
	for _, c := range chunks {
		m.Required[c] = true
	}
	return m
}
func (m *WANTransfer) AcceptMetadata(approved bool) bool {
	if !approved {
		return false
	}
	m.Metadata = true
	return true
}
func (m *WANTransfer) Chunk(id string, verified bool) bool {
	if !m.Metadata || !m.Required[id] || !verified {
		return false
	}
	m.Verified[id] = true
	return true
}
func (m *WANTransfer) CommitReady(wholeHash bool) bool {
	if !m.Metadata || !wholeHash {
		return false
	}
	for c := range m.Required {
		if !m.Verified[c] {
			return false
		}
	}
	m.Ready = true
	return true
}
func (m *WANTransfer) SendReceipt(authenticated bool) bool {
	if !m.Ready || !authenticated {
		return false
	}
	m.Receipt = true
	return true
}
func (m *WANTransfer) Switch(route, pin string) bool {
	if pin != m.Pin || (route != "quic" && route != "tcp" && route != "relay") {
		return false
	}
	m.Route = route
	return true
}
func (m *WANTransfer) Restart() { m.Route = "" }
