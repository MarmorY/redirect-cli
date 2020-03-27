package redirector

import (
	"fmt"
	"net"

	"github.com/apex/log"
)

// RedirectedConnectionHandler declares signatur for handler function
type RedirectedConnectionHandler func(net.Conn, *RedirectRule) error

// RedirectRule describes how to redirect IP packets
type RedirectRule struct {
	protocolPort     uint16
	targetPort       uint16
	targetIP         *net.IP
	excludedNetworks []*net.IPNet
	proxyAddress     string

	redirectedConnectionHandler RedirectedConnectionHandler
	redirectListener            net.Listener
	portToIPMapping             []*net.IP
}

// NewRedirectRule creates a new RedirectRule from provided arguments
func NewRedirectRule(protocolPort, targetPort int, targetIP *net.IP, proxyAddress string, connectionHandler RedirectedConnectionHandler, excludedNetworks []*net.IPNet) (*RedirectRule, error) {

	rule := &RedirectRule{
		protocolPort:                uint16(protocolPort),
		targetPort:                  uint16(targetPort),
		targetIP:                    targetIP,
		proxyAddress:                proxyAddress,
		redirectedConnectionHandler: connectionHandler,
		portToIPMapping:             make([]*net.IP, MaxTCPPort),
		excludedNetworks:            excludedNetworks,
	}

	return rule, nil
}

// GetTargetIP returns rule's target IP
func (r *RedirectRule) GetTargetIP() *net.IP {
	return r.targetIP
}

// GetProxyAddress returns target proxies address
func (r *RedirectRule) GetProxyAddress() string {
	return r.proxyAddress
}

// GetTargetPort returns rule's target port
func (r *RedirectRule) GetTargetPort() uint16 {
	return r.targetPort
}

// GetProtocolPort returns rule's protocol port
func (r *RedirectRule) GetProtocolPort() uint16 {
	return r.protocolPort
}

func (r *RedirectRule) listenForConnection() {
	listenAdress := fmt.Sprintf("%s:%d", r.targetIP, r.targetPort)
	log.Infof("Listening on %s for incoming connections.", listenAdress)
	ln, err := net.Listen("tcp", listenAdress)
	if err != nil {
		log.Fatalf("Error opening listener: %v", err)
	}

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Errorf("Error accepting connection: %v", err)
			break
		} else {
			go r.redirectedConnectionHandler(conn, r)
		}
	}

}

func (r *RedirectRule) isExcludedIP(ip net.IP) bool {
	if r.excludedNetworks != nil {
		for _, block := range r.excludedNetworks {
			if block.Contains(ip) {
				return true
			}
		}
	}
	return false
}
