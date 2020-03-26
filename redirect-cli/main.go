package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"sync"

	redirector "github.com/Neothorn23/redirecttoproxy"
	"github.com/apex/log"
)

const (
	defaultHTTPPort          = 80
	defaultHTTPRedirectPort  = 40080
	defaultHTTPSPort         = 443
	defaultHTTPSRedirectPort = 40443

	proxyBufferSize = 4096
)

var (
	version string
)

func main() {
	fmt.Printf("redirecttoproxy %s\n\n", version)

	proxyAddress := flag.String("proxy", "", "destination proxy: <ip addr>:<port>")
	redirectAddressString := flag.String("listen", "", "ip adress to list on.")
	httpPort := flag.Int("httpPort", defaultHTTPPort, "source port for redirected http connections")
	httpTargetPort := flag.Int("httpRedirectPort", defaultHTTPRedirectPort, "target port for redirected http connections")
	httpsPort := flag.Int("httpsPort", defaultHTTPSPort, "source port for redirected http connections")
	httpsTargetPort := flag.Int("httpsRedirectPort", defaultHTTPSRedirectPort, "target port for redirected http connections")
	debug := flag.Bool("v", false, "Verbose output")

	flag.Parse()

	flagsError := false

	if *proxyAddress == "" {
		fmt.Println("Parameter \"proxy\" is not set.")
		flagsError = true
	}

	var redirectAddress net.IP
	if *redirectAddressString == "" {
		redirectAddress = getOutboundIP()
	} else {
		redirectAddress = net.ParseIP(*redirectAddressString)
		if redirectAddress == nil {
			fmt.Printf("\"%s\" is not a valid IP address.\n", *redirectAddressString)
			flagsError = true
		}
	}

	if *httpPort <= 0 || *httpPort >= redirector.MaxTCPPort {
		fmt.Printf("The value for httpPort should be between 1 an %d", redirector.MaxTCPPort-1)
	}

	if *httpTargetPort <= 0 || *httpTargetPort >= redirector.MaxTCPPort {
		fmt.Printf("The value for httpProxyPort should be between 1 an %d", redirector.MaxTCPPort-1)
	}

	if flagsError {
		flag.PrintDefaults()
		os.Exit(1)
	}

	if *debug {
		log.SetLevel(log.DebugLevel)
	}
	log.Debug("Verbos output is enabled.")

	fmt.Println("Starting using following settings:")
	fmt.Printf("  target IP         = %s\n", redirectAddress)
	fmt.Printf("  http port         = %d\n", *httpPort)
	fmt.Printf("  http target port  = %d\n", *httpTargetPort)
	fmt.Printf("  https port        = %d\n", *httpsPort)
	fmt.Printf("  https target port = %d\n", *httpsTargetPort)
	fmt.Printf("  proxy address     = %s\n", *proxyAddress)

	httpRule, err := redirector.NewRedirectRule(*httpPort, *httpTargetPort, &redirectAddress, *proxyAddress, redirector.SendRedirectedHTTPConnectionToHTTPProxy)
	if err != nil {
		log.Errorf("Error creating http redirect rule: %s", err)
		os.Exit(1)
	}

	httpsRule, err := redirector.NewRedirectRule(*httpsPort, *httpsTargetPort, &redirectAddress, *proxyAddress, redirector.SendRedirectedHTTPSConnectionToHTTPProxy)
	if err != nil {
		log.Errorf("Error creating https redirect rule: %s", err)
		os.Exit(1)
	}

	redirector, err := redirector.NewRedirector([]*redirector.RedirectRule{httpRule, httpsRule})
	if err != nil {
		log.Errorf("Error creating redirector: %s", err)
		os.Exit(2)
	}
	defer redirector.Close()

	waitForCtrlC()
	fmt.Println("... stopped")

}

func getOutboundIP() net.IP {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		log.Errorf("Error obtaining outbound IP: ", err)
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)

	return net.ParseIP(localAddr.IP.String())
}

func waitForCtrlC() {
	var endWaiter sync.WaitGroup
	endWaiter.Add(1)
	var signalChannel chan os.Signal
	signalChannel = make(chan os.Signal, 1)
	signal.Notify(signalChannel, os.Interrupt)
	go func() {
		<-signalChannel
		endWaiter.Done()
	}()
	endWaiter.Wait()
}
