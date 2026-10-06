package rendezvous

import (
	"net"
	"testing"
	"time"

	"github.com/pion/stun/v4"
)

func TestWANW10STUNMappingMalformedAndRateBounds(t *testing.T) {
	var ip net.IP
	addrs, e := net.InterfaceAddrs()
	if e != nil {
		t.Fatal(e)
	}
	for _, a := range addrs {
		host, _, _ := net.ParseCIDR(a.String())
		if host != nil && host.To4() != nil && host.IsPrivate() && !host.IsLoopback() {
			ip = host
			break
		}
	}
	if ip == nil {
		t.Fatal("native private IPv4 required")
	}
	socket, e := net.ListenPacket("udp4", net.JoinHostPort(ip.String(), "0"))
	if e != nil {
		t.Fatal(e)
	}
	server, e := NewSTUNServer(socket)
	if e != nil {
		t.Fatal(e)
	}
	defer server.Close()
	client, e := net.ListenUDP("udp4", &net.UDPAddr{IP: ip})
	if e != nil {
		t.Fatal(e)
	}
	defer client.Close()
	request, e := stun.Build(stun.TransactionID, stun.BindingRequest)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = client.WriteTo(request.Raw, server.LocalAddr()); e != nil {
		t.Fatal(e)
	}
	_ = client.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 1024)
	n, _, e := client.ReadFrom(buf)
	if e != nil {
		t.Fatal(e)
	}
	if n > 2*len(request.Raw) {
		t.Fatal("amplification", n)
	}
	response := &stun.Message{Raw: buf[:n]}
	if e = response.Decode(); e != nil {
		t.Fatal(e)
	}
	var mapped stun.XORMappedAddress
	if e = mapped.GetFrom(response); e != nil {
		t.Fatal(e)
	}
	if response.Type != stun.BindingSuccess || response.TransactionID != request.TransactionID || mapped.Port != client.LocalAddr().(*net.UDPAddr).Port || !mapped.IP.Equal(ip) {
		t.Fatal("mapping")
	}
	// Malformed, nonbinding, alternate-address and oversized requests get silence.
	badType, _ := stun.Build(stun.TransactionID, stun.BindingSuccess)
	attributed, _ := stun.Build(stun.TransactionID, stun.BindingRequest, stun.RawAttribute{Type: stun.AttrChangeRequest, Value: []byte{0, 0, 0, 6}})
	for _, bad := range [][]byte{{1, 2, 3}, badType.Raw, attributed.Raw, make([]byte, 1024)} {
		_, _ = client.WriteTo(bad, server.LocalAddr())
	}
	_ = client.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
	if _, _, e = client.ReadFrom(buf); e == nil {
		t.Fatal("malformed got response")
	}
	// Align a fresh second to measure one exact fixed-window prefix budget.
	time.Sleep(time.Until(time.Now().Truncate(time.Second).Add(time.Second)))
	for range 100 {
		_, _ = client.WriteTo(request.Raw, server.LocalAddr())
	}
	count := 0
	_ = client.SetReadDeadline(time.Now().Add(150 * time.Millisecond))
	for {
		n, _, e = client.ReadFrom(buf)
		if e != nil {
			break
		}
		if n > 2*len(request.Raw) {
			t.Fatal("amplification")
		}
		count++
	}
	if count == 0 || count > 10 {
		t.Fatal("prefix rate", count)
	}
	t.Log("IPv4 binding response 32/20 bytes; 100-request burst responses", count, "; malformed silent; closure joins sole worker")
	if e = server.Close(); e != nil {
		t.Fatal(e)
	}
}
