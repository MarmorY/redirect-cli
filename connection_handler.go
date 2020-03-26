package redirector

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/apex/log"
	"github.com/inconshreveable/go-vhost"
)

// SendRedirectedHTTPConnectionToHTTPProxy sends redirected http connections to a proxy
func SendRedirectedHTTPConnectionToHTTPProxy(clientConn net.Conn, rule *RedirectRule) error {
	log.Debugf("Recieved HTTP connection")
	defer clientConn.Close()
	proxyConn, err := net.DialTimeout("tcp", rule.GetProxyAddress(), 10*time.Second)
	if err != nil {
		return fmt.Errorf("Error opening connection to proxy: %v", err)
	}
	defer proxyConn.Close()

	clientReader := bufio.NewReader(clientConn)

	request, err := http.ReadRequest(clientReader)
	if err != nil {
		return fmt.Errorf("Error parsing HTTP request: %v", err)
	}

	//Send request as proxy request
	request.URL.Scheme = "http"
	request.URL.Host = request.Host
	request.WriteProxy(proxyConn)

	handleRemainingCommunication(clientConn, proxyConn)

	return nil
}

// SendRedirectedHTTPSConnectionToHTTPProxy sends redirected secure http connections to a proxy
func SendRedirectedHTTPSConnectionToHTTPProxy(clientConn net.Conn, rule *RedirectRule) error {
	log.Debugf("Recieved HTTPS connection")
	defer clientConn.Close()
	tlsConn, err := vhost.TLS(clientConn)
	if err != nil {
		return fmt.Errorf("Failed handling TLS connection - %s", err.Error())
	}
	defer tlsConn.Free()

	dstServer := tlsConn.Host()
	if dstServer == "" {
		return errors.New("Cannot get host from SNI, so fallback trying do to reverse lookup for original IP address")
	}

	proxyConn, err := net.DialTimeout("tcp", rule.GetProxyAddress(), 10*time.Second)
	if err != nil {
		return fmt.Errorf("Cannot connect via proxy %s to %s: %s", rule.GetProxyAddress(), dstServer, err)
	}
	defer proxyConn.Close()

	//Initiate Tunnel
	reqURL, err := url.Parse("http://" + fmt.Sprintf("%s:%d", dstServer, rule.GetProtocolPort()))
	if err != nil {
		return fmt.Errorf("Error parsing destination server URL: %s", err)
	}
	req, err := http.NewRequest(http.MethodConnect, reqURL.String(), nil)
	if err != nil {
		return fmt.Errorf("Cannot create tunnel request: %s", err)
	}
	req.Close = false
	req.Header.Set("User-Agent", "RedirectToProxy")

	err = req.Write(proxyConn)
	if err != nil {
		return fmt.Errorf("Error sending tunnel request: %s", err)
	}

	resp, err := http.ReadResponse(bufio.NewReader(proxyConn), req)
	if err != nil {
		resp.Body.Close()
		return fmt.Errorf("Error reading response to tunnel request: %s", err)
	}

	resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("Proxy Server returned HTTP result [%d]", resp.StatusCode)
	}

	//Write ClientHello to real destination because we have already read it
	ch := tlsConn.ClientHelloMsg.Raw
	chSize := len(ch)
	chHeader := []byte{0x16, 0x03, 0x01, byte(chSize >> 8), byte(chSize)}
	chRecord := append(chHeader, ch...)
	proxyConn.Write(chRecord)

	handleRemainingCommunication(clientConn, proxyConn)

	return nil
}

func handleRemainingCommunication(clientConn net.Conn, proxyConn net.Conn) {

	//Handle data transfer until connection is no more needed
	var wg sync.WaitGroup
	wg.Add(1)
	go transfer(proxyConn, clientConn, &wg)
	wg.Add(1)
	go transfer(clientConn, proxyConn, &wg)
	wg.Wait()

}

func transfer(destination io.Writer, source io.Reader, wg *sync.WaitGroup) {
	defer wg.Done()
	io.Copy(destination, source)
}
