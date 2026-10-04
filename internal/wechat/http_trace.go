package wechat

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"sync"
)

type HTTPConnectTrace struct {
	Network string `json:"network,omitempty"`
	Address string `json:"address,omitempty"`
	Error   string `json:"error,omitempty"`
}

type HTTPNetworkTrace struct {
	ProxyURL          string             `json:"proxy_url,omitempty"`
	ProxyError        string             `json:"proxy_error,omitempty"`
	DNSHost           string             `json:"dns_host,omitempty"`
	DNSAddresses      []string           `json:"dns_addresses,omitempty"`
	DNSError          string             `json:"dns_error,omitempty"`
	Connects          []HTTPConnectTrace `json:"connects,omitempty"`
	LocalAddress      string             `json:"local_address,omitempty"`
	RemoteAddress     string             `json:"remote_address,omitempty"`
	ConnectionReused  bool               `json:"connection_reused,omitempty"`
	ConnectionWasIdle bool               `json:"connection_was_idle,omitempty"`
	ConnectionIdleMS  int64              `json:"connection_idle_ms,omitempty"`
	TLSServerName     string             `json:"tls_server_name,omitempty"`
	TLSVersion        string             `json:"tls_version,omitempty"`
	TLSHandshakeError string             `json:"tls_handshake_error,omitempty"`
	RequestWriteError string             `json:"request_write_error,omitempty"`
}

type httpNetworkTraceRecorder struct {
	mu    sync.Mutex
	trace HTTPNetworkTrace
}

func traceRequestNetwork(req *http.Request, client *http.Client) (*http.Request, func() HTTPNetworkTrace) {
	recorder := &httpNetworkTraceRecorder{}
	proxyURL, proxyErr := proxyForRequest(req, client)
	recorder.trace.ProxyURL = proxyURL
	recorder.trace.ProxyError = proxyErr

	clientTrace := &httptrace.ClientTrace{
		DNSStart: func(info httptrace.DNSStartInfo) {
			recorder.mu.Lock()
			recorder.trace.DNSHost = info.Host
			recorder.mu.Unlock()
		},
		DNSDone: func(info httptrace.DNSDoneInfo) {
			recorder.mu.Lock()
			for _, address := range info.Addrs {
				recorder.trace.DNSAddresses = append(recorder.trace.DNSAddresses, address.String())
			}
			if info.Err != nil {
				recorder.trace.DNSError = info.Err.Error()
			}
			recorder.mu.Unlock()
		},
		ConnectDone: func(network, address string, err error) {
			connection := HTTPConnectTrace{Network: network, Address: address}
			if err != nil {
				connection.Error = err.Error()
			}
			recorder.mu.Lock()
			recorder.trace.Connects = append(recorder.trace.Connects, connection)
			recorder.mu.Unlock()
		},
		GotConn: func(info httptrace.GotConnInfo) {
			recorder.mu.Lock()
			if info.Conn != nil {
				recorder.trace.LocalAddress = info.Conn.LocalAddr().String()
				recorder.trace.RemoteAddress = info.Conn.RemoteAddr().String()
			}
			recorder.trace.ConnectionReused = info.Reused
			recorder.trace.ConnectionWasIdle = info.WasIdle
			recorder.trace.ConnectionIdleMS = info.IdleTime.Milliseconds()
			recorder.mu.Unlock()
		},
		TLSHandshakeDone: func(state tls.ConnectionState, err error) {
			recorder.mu.Lock()
			recorder.trace.TLSServerName = state.ServerName
			recorder.trace.TLSVersion = tlsVersionName(state.Version)
			if err != nil {
				recorder.trace.TLSHandshakeError = err.Error()
			}
			recorder.mu.Unlock()
		},
		WroteRequest: func(info httptrace.WroteRequestInfo) {
			if info.Err == nil {
				return
			}
			recorder.mu.Lock()
			recorder.trace.RequestWriteError = info.Err.Error()
			recorder.mu.Unlock()
		},
	}

	tracedRequest := req.WithContext(httptrace.WithClientTrace(req.Context(), clientTrace))
	return tracedRequest, recorder.snapshot
}

func (r *httpNetworkTraceRecorder) snapshot() HTTPNetworkTrace {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := r.trace
	result.DNSAddresses = append([]string(nil), r.trace.DNSAddresses...)
	result.Connects = append([]HTTPConnectTrace(nil), r.trace.Connects...)
	return result
}

func proxyForRequest(req *http.Request, client *http.Client) (string, string) {
	var proxy func(*http.Request) (*url.URL, error)
	if client != nil && client.Transport != nil {
		if transport, ok := client.Transport.(*http.Transport); ok {
			if transport.Proxy == nil {
				return "", ""
			}
			proxy = transport.Proxy
		} else {
			return "", ""
		}
	}
	if proxy == nil {
		proxy = http.ProxyFromEnvironment
	}
	proxyURL, err := proxy(req)
	if err != nil {
		return "", err.Error()
	}
	if proxyURL == nil {
		return "", ""
	}
	return proxyURL.String(), ""
}

func tlsVersionName(version uint16) string {
	switch version {
	case tls.VersionTLS10:
		return "TLS 1.0"
	case tls.VersionTLS11:
		return "TLS 1.1"
	case tls.VersionTLS12:
		return "TLS 1.2"
	case tls.VersionTLS13:
		return "TLS 1.3"
	case 0:
		return ""
	default:
		return fmt.Sprintf("0x%04x", version)
	}
}
