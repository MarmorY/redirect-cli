package redirector

import (
	"fmt"
	"net"
	"strings"

	"github.com/apex/log"
	"github.com/williamfhe/godivert"
)

const (
	// MaxTCPPort max TCP port count
	MaxTCPPort = 65536
)

var (
	privateIPBlocks []*net.IPNet
)

// Redirector manages and implements a group of RedirectRules
type Redirector struct {
	redirectRules   []*RedirectRule
	portLookupTable [MaxTCPPort]*RedirectRule
	handle          *godivert.WinDivertHandle
}

func init() {
	for _, cidr := range []string{
		"127.0.0.0/8",    // IPv4 loopback
		"10.0.0.0/8",     // RFC1918
		"172.16.0.0/12",  // RFC1918
		"192.168.0.0/16", // RFC1918
		"169.254.0.0/16", // RFC3927 link-local
		"::1/128",        // IPv6 loopback
		"fe80::/10",      // IPv6 link-local
		"fc00::/7",       // IPv6 unique local addr
	} {
		_, block, err := net.ParseCIDR(cidr)
		if err != nil {
			panic(fmt.Errorf("parse error on %q: %v", cidr, err))
		}
		privateIPBlocks = append(privateIPBlocks, block)
	}
}

// NewRedirector creates a new IPRedirector with a set of redirection rules
func NewRedirector(rules []*RedirectRule) (*Redirector, error) {
	redirector := &Redirector{
		redirectRules: rules,
	}

	return redirector, redirector.initialize()
}

// initializes the IPRedirector
func (r *Redirector) initialize() error {

	var filterBuilder strings.Builder
	filterBuilder.WriteString("tcp and (")
	for idx, rule := range r.redirectRules {
		if idx > 0 {
			filterBuilder.WriteString(" or ")
		}
		filterBuilder.WriteString(fmt.Sprintf("tcp.DstPort == %d or tcp.SrcPort == %d",
			rule.protocolPort,
			rule.targetPort))

		r.portLookupTable[rule.protocolPort] = rule
		r.portLookupTable[rule.targetPort] = rule
	}
	filterBuilder.WriteString(")")

	packetFilter := filterBuilder.String()

	log.Debugf("Packet filter expression: \"%s\"", packetFilter)

	winDivert, err := godivert.NewWinDivertHandle(packetFilter)
	if err != nil {
		return err
	}
	r.handle = winDivert

	packetChan, err := r.handle.Packets()
	if err != nil {
		r.handle.Close()
		return err
	}

	go r.checkAndRedirectHTTPPacket(winDivert, packetChan)

	r.initializeRedirectedConnectionListeners()

	return nil
}

// Close closes the redirector
func (r *Redirector) Close() {
	r.handle.Close()
}

func (r *Redirector) checkAndRedirectHTTPPacket(wd *godivert.WinDivertHandle, packetChan <-chan *godivert.Packet) {
	log.Info("Redirecting ip packets ...")

	for packet := range packetChan {

		go func(packet *godivert.Packet) {

			dstIP := packet.DstIP()
			dstPort, _ := packet.DstPort()
			srcPort, _ := packet.SrcPort()

			rule := r.portLookupTable[dstPort]
			if rule != nil {
				redirectPacketToProxy(wd, packet, srcPort, dstIP, dstPort, rule)
			} else {
				rule := r.portLookupTable[srcPort]
				if rule != nil {
					redirectPacketToClient(wd, packet, srcPort, dstPort, rule)
				}
			}

			wd.Send(packet)

		}(packet)
	}
}

func redirectPacketToProxy(wd *godivert.WinDivertHandle, packet *godivert.Packet, srcPort uint16, originalDstIP net.IP, originalDstPort uint16, rule *RedirectRule) {
	if !isPrivateIP(originalDstIP) {
		log.Debugf("Redirecting outgoing package from %s:%d to %s:%d", originalDstIP, originalDstPort, *rule.GetTargetIP(), rule.GetTargetPort())
		rule.portToIPMapping[srcPort] = &originalDstIP
		packet.SetDstPort(rule.GetTargetPort())
		packet.SetDstIP(*rule.GetTargetIP())
		wd.HelperCalcChecksum(packet)
	}
}

func redirectPacketToClient(wd *godivert.WinDivertHandle, packet *godivert.Packet, originalSrcPort, dstPort uint16, rule *RedirectRule) {
	originalSrcIP := packet.SrcIP()
	newSrcIP := *rule.portToIPMapping[dstPort]
	newSrcPort := rule.GetProtocolPort()
	log.Debugf("Redirecting incoming package from %s:%d to %s:%d", originalSrcIP, originalSrcPort, newSrcIP, newSrcPort)
	packet.SetSrcPort(newSrcPort)
	packet.SetSrcIP(newSrcIP)
	wd.HelperCalcChecksum(packet)
}

func (r *Redirector) initializeRedirectedConnectionListeners() {

	for _, rule := range r.redirectRules {
		go rule.listenForConnection()
	}

}

func isPrivateIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}

	for _, block := range privateIPBlocks {
		if block.Contains(ip) {
			return true
		}
	}
	return false
}
