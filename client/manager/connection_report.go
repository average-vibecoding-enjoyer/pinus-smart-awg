package manager

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/amnezia-vpn/amneziawg-windows-client/smart"
	"golang.org/x/net/proxy"
	"golang.org/x/sys/windows"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

type ConnectionSnapshot struct {
	Name     string
	Settings smart.RoutingSettings
	Engine   string
}

func (s *ManagerService) ConnectionSnapshot() (ConnectionSnapshot, error) {
	smartLifecycleLock.Lock()
	defer smartLifecycleLock.Unlock()
	smartProcessLock.Lock()
	state := smartProcess
	smartProcessLock.Unlock()
	if state != nil {
		return ConnectionSnapshot{state.tunnelName, state.settings, "smart"}, nil
	}
	states, err := s.nativeTunnelStates()
	if err != nil {
		return ConnectionSnapshot{}, err
	}
	for name, state := range states {
		if state == TunnelStarted {
			return ConnectionSnapshot{name, smart.DefaultSettings(), "native"}, nil
		}
	}
	return ConnectionSnapshot{}, nil
}

func prepareDiagnosticListener() (int, string, error) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return 0, "", err
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	// If the port is acquired by someone else in this small handoff window,
	// the engine fails startup. We never trust an unauthenticated listener.
	var secret [32]byte
	if _, err = rand.Read(secret[:]); err != nil {
		return 0, "", err
	}
	return port, hex.EncodeToString(secret[:]), nil
}

func traceIP(body string) (string, error) {
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "ip=") {
			ip := net.ParseIP(strings.TrimSpace(strings.TrimPrefix(line, "ip=")))
			if ip != nil {
				return ip.String(), nil
			}
		}
	}
	return "", errors.New("ответ не содержит IP-адрес")
}

func probeHTTP(ctx context.Context, client *http.Client, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	response, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 16*1024+1))
	if err != nil {
		return "", err
	}
	if len(data) > 16*1024 {
		return "", errors.New("слишком большой ответ")
	}
	return traceIP(string(data))
}

func (s *ManagerService) ProbeConnection() (string, error) {
	if s.elevatedToken == 0 {
		return "", windows.ERROR_ACCESS_DENIED
	}
	snapshot, err := s.ConnectionSnapshot()
	if err != nil {
		return "", err
	}
	if snapshot.Name == "" {
		return "VPN не подключён. Сетевой запрос не выполнялся.", nil
	}
	if snapshot.Engine == "native" {
		config, err := s.RuntimeConfig(snapshot.Name)
		if err != nil {
			return "", err
		}
		result := "Native AWG: данные работающего туннеля\n"
		for i, peer := range config.Peers {
			handshake := "ещё не было"
			if peer.LastHandshakeTime != 0 {
				handshake = time.Unix(0, int64(peer.LastHandshakeTime)).Local().Format(time.RFC3339)
			}
			result += fmt.Sprintf("Пир %d: handshake %s, принято %d B, отправлено %d B\n", i+1, handshake, peer.RxBytes, peer.TxBytes)
		}
		return result + "Счётчики и handshake не доказывают доступность сайта или отсутствие утечек. Активная HTTPS-проверка здесь доступна для smart-режима.", nil
	}
	smartProcessLock.Lock()
	state := smartProcess
	if state == nil || state.tunnelName != snapshot.Name {
		smartProcessLock.Unlock()
		return "", errors.New("подключение изменилось")
	}
	port, password := state.probePort, state.probePassword
	smartProcessLock.Unlock()
	dialer, err := proxy.SOCKS5("tcp", fmt.Sprintf("127.0.0.1:%d", port), &proxy.Auth{User: "pinus-probe", Password: password}, &net.Dialer{Timeout: 6 * time.Second})
	if err != nil {
		return "", err
	}
	contextDialer, ok := dialer.(proxy.ContextDialer)
	if !ok {
		return "", errors.New("SOCKS не поддерживает тайм-аут")
	}
	transport := &http.Transport{DialContext: contextDialer.DialContext, TLSHandshakeTimeout: 6 * time.Second, ResponseHeaderTimeout: 6 * time.Second, DisableKeepAlives: true}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	ctx, cancel := context.WithTimeout(context.Background(), 24*time.Second)
	defer cancel()
	var report strings.Builder
	report.WriteString("HTTPS через AWG-выход smart-движка (TLS проверяется):\n")
	for _, probe := range []struct{ name, url string }{{"IPv4", "https://1.1.1.1/cdn-cgi/trace"}, {"IPv6", "https://[2606:4700:4700::1111]/cdn-cgi/trace"}, {"DNS + HTTPS", "https://www.cloudflare.com/cdn-cgi/trace"}} {
		started := time.Now()
		ip, err := probeHTTP(ctx, client, probe.url)
		if err != nil {
			fmt.Fprintf(&report, "%s: не подтверждено (%v)\n", probe.name, err)
		} else {
			fmt.Fprintf(&report, "%s: выход %s, %s\n", probe.name, ip, time.Since(started).Round(time.Millisecond))
		}
	}
	smartProcessLock.Lock()
	same := smartProcess == state
	smartProcessLock.Unlock()
	if !same {
		report.WriteString("Подключение сменилось во время проверки. Повторите проверку.\n")
	}
	report.WriteString("Проверен принудительный AWG-выход. Эти запросы не проверяют маршрут каждого приложения, браузерный DoH/WebRTC и прямые исключения.")
	return report.String(), nil
}

func IPCClientConnectionSnapshot() (ConnectionSnapshot, error) {
	rpcMutex.Lock()
	defer rpcMutex.Unlock()
	var result ConnectionSnapshot
	var errText string
	if err := rpcEncoder.Encode(ConnectionSnapshotMethodType); err != nil {
		return result, err
	}
	if err := rpcDecoder.Decode(&result); err != nil {
		return result, err
	}
	if err := rpcDecoder.Decode(&errText); err != nil {
		return result, err
	}
	if errText != "" {
		return result, errors.New(errText)
	}
	return result, nil
}
func IPCClientProbeConnection() (string, error) {
	rpcMutex.Lock()
	defer rpcMutex.Unlock()
	var result, errText string
	if err := rpcEncoder.Encode(ProbeConnectionMethodType); err != nil {
		return "", err
	}
	if err := rpcDecoder.Decode(&result); err != nil {
		return "", err
	}
	if err := rpcDecoder.Decode(&errText); err != nil {
		return "", err
	}
	if errText != "" {
		return result, errors.New(errText)
	}
	return result, nil
}
