//go:build windows

package discovery

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode"

	"github.com/miekg/dns"
	"golang.org/x/net/ipv4"
)

const (
	mdnsPort        = 5353
	cacheFlush      = uint16(1 << 15)
	announcementTTL = uint32(120)
)

var mdnsDestination = &net.UDPAddr{IP: net.IPv4(224, 0, 0, 251), Port: mdnsPort}

type interfaceAddress struct {
	iface net.Interface
	ip    net.IP
}

type windowsAdvertisement struct {
	name      string
	port      int
	text      []string
	host      string
	addresses []interfaceAddress
	conn      *ipv4.PacketConn
	cancel    context.CancelFunc
	done      sync.WaitGroup
	writeMu   sync.Mutex
}

// Register uses an explicit-interface responder on Windows. Windows' own mDNS
// service also owns UDP 5353, and wildcard multicast writes with an interface
// control message can be silently discarded. Selecting the interface on the
// socket and sending ordinary multicast packets avoids that failure mode.
func Register(name string, port int, text []string) (Advertisement, error) {
	addresses, err := multicastAddresses()
	if err != nil {
		return nil, err
	}
	listen := net.ListenConfig{Control: func(network, address string, raw syscall.RawConn) error {
		var optionErr error
		if err := raw.Control(func(fd uintptr) {
			optionErr = syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
		}); err != nil {
			return err
		}
		return optionErr
	}}
	packet, err := listen.ListenPacket(context.Background(), "udp4", fmt.Sprintf("0.0.0.0:%d", mdnsPort))
	if err != nil {
		return nil, fmt.Errorf("listen for mDNS: %w", err)
	}
	conn := ipv4.NewPacketConn(packet)
	joined := 0
	seen := make(map[int]bool)
	for _, address := range addresses {
		if seen[address.iface.Index] {
			continue
		}
		seen[address.iface.Index] = true
		if err := conn.JoinGroup(&address.iface, mdnsDestination); err == nil {
			joined++
		}
	}
	if joined == 0 {
		conn.Close()
		return nil, fmt.Errorf("join mDNS multicast group on every active interface")
	}
	_ = conn.SetControlMessage(ipv4.FlagInterface, true)
	_ = conn.SetMulticastTTL(255)
	ctx, cancel := context.WithCancel(context.Background())
	advertisement := &windowsAdvertisement{
		name: name, port: port, text: append([]string(nil), text...),
		host: localHostName(), addresses: addresses, conn: conn, cancel: cancel,
	}
	advertisement.done.Add(2)
	go advertisement.announceLoop(ctx)
	go advertisement.readLoop(ctx)
	return advertisement, nil
}

func multicastAddresses() ([]interfaceAddress, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var result []interfaceAddress
	for _, iface := range interfaces {
		if iface.Flags&(net.FlagUp|net.FlagMulticast) != net.FlagUp|net.FlagMulticast || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addresses, _ := iface.Addrs()
		for _, address := range addresses {
			ip, _, err := net.ParseCIDR(address.String())
			if err == nil && ip.To4() != nil && !ip.IsLoopback() && !ip.IsUnspecified() {
				result = append(result, interfaceAddress{iface: iface, ip: ip.To4()})
			}
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("no active IPv4 multicast interface")
	}
	return result, nil
}

func localHostName() string {
	host, _ := os.Hostname()
	var clean strings.Builder
	for _, r := range host {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' {
			clean.WriteRune(r)
		} else if clean.Len() > 0 {
			clean.WriteByte('-')
		}
	}
	value := strings.Trim(clean.String(), "-")
	if value == "" {
		value = "surf"
	}
	return value + ".local."
}

func escapeLabel(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	return strings.ReplaceAll(value, `.`, `\.`)
}

func (a *windowsAdvertisement) message(ip net.IP, ttl uint32) ([]byte, error) {
	service := "_surf._tcp.local."
	instance := escapeLabel(a.name) + "." + service
	class := dns.ClassINET
	if ttl > 0 {
		class |= int(cacheFlush)
	}
	message := new(dns.Msg)
	message.Response = true
	message.Authoritative = true
	message.Compress = true
	message.Answer = []dns.RR{
		&dns.PTR{Hdr: dns.RR_Header{Name: service, Rrtype: dns.TypePTR, Class: dns.ClassINET, Ttl: ttl}, Ptr: instance},
		&dns.SRV{Hdr: dns.RR_Header{Name: instance, Rrtype: dns.TypeSRV, Class: uint16(class), Ttl: ttl}, Port: uint16(a.port), Target: a.host},
		&dns.TXT{Hdr: dns.RR_Header{Name: instance, Rrtype: dns.TypeTXT, Class: uint16(class), Ttl: ttl}, Txt: a.text},
		&dns.A{Hdr: dns.RR_Header{Name: a.host, Rrtype: dns.TypeA, Class: uint16(class), Ttl: ttl}, A: ip.To4()},
	}
	return message.Pack()
}

func (a *windowsAdvertisement) announce(ttl uint32) {
	a.writeMu.Lock()
	defer a.writeMu.Unlock()
	for _, address := range a.addresses {
		data, err := a.message(address.ip, ttl)
		if err != nil || a.conn.SetMulticastInterface(&address.iface) != nil {
			continue
		}
		_, _ = a.conn.WriteTo(data, nil, mdnsDestination)
	}
}

func (a *windowsAdvertisement) announceLoop(ctx context.Context) {
	defer a.done.Done()
	a.announce(announcementTTL)
	initial := time.NewTimer(time.Second)
	defer initial.Stop()
	select {
	case <-ctx.Done():
		return
	case <-initial.C:
		a.announce(announcementTTL)
	}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.announce(announcementTTL)
		}
	}
}

func (a *windowsAdvertisement) readLoop(ctx context.Context) {
	defer a.done.Done()
	buffer := make([]byte, 64*1024)
	for {
		n, _, _, err := a.conn.ReadFrom(buffer)
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				continue
			}
		}
		var query dns.Msg
		if query.Unpack(buffer[:n]) != nil || query.Response {
			continue
		}
		for _, question := range query.Question {
			if strings.EqualFold(question.Name, "_surf._tcp.local.") {
				go a.announce(announcementTTL)
				break
			}
		}
	}
}

func (a *windowsAdvertisement) Shutdown() {
	a.announce(0)
	a.cancel()
	_ = a.conn.Close()
	a.done.Wait()
}
