//go:build windows

package directdns_test

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/netip"
	"sync/atomic"
	"testing"
	"time"

	"github.com/amnezia-vpn/amneziawg-windows-client/smart"
	"github.com/amnezia-vpn/amneziawg-windows/conf"
	mDNS "github.com/miekg/dns"
	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/common/dialer"
	C "github.com/sagernet/sing-box/constant"
	boxDNS "github.com/sagernet/sing-box/dns"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	M "github.com/sagernet/sing/common/metadata"
	"github.com/sagernet/sing/service"
)

// All DNS exchanges and final connections remain in memory. No box.New,
// listeners, OS sockets, network-interface monitor, TUN or service is created.
// The test-only detour replaces the terminal network dial while preserving
// the engine's real resolver selection, resolve dialer, DNS rules and client.
func TestGeneratedDirectOutboundUsesDirectDNS(t *testing.T) {
	for _, target := range []struct{ service, domain string }{
		{smart.VKServiceID, "vk.ru"},
		{smart.RussianServiceID, "yandex.ru"},
	} {
		t.Run(target.service, func(t *testing.T) {
			config, err := conf.FromWgQuick(syntheticProfile, "test-only")
			if err != nil {
				t.Fatal(err)
			}
			settings := smart.RoutingSettings{Mode: smart.ModeAll}.WithServiceVPN(target.service, false)
			encoded, err := smart.BuildConfig(config, settings)
			if err != nil {
				t.Fatal(err)
			}
			var document struct {
				DNS   json.RawMessage `json:"dns"`
				Route struct {
					Resolver string `json:"default_domain_resolver"`
				} `json:"route"`
				Outbounds []json.RawMessage `json:"outbounds"`
			}
			if err := json.Unmarshal(encoded, &document); err != nil {
				t.Fatal(err)
			}
			var directOptions option.DirectOutboundOptions
			found := false
			for _, outbound := range document.Outbounds {
				var identity struct {
					Tag string `json:"tag"`
				}
				if err := json.Unmarshal(outbound, &identity); err != nil {
					t.Fatal(err)
				}
				if identity.Tag == "direct" {
					if err := json.Unmarshal(outbound, &directOptions); err != nil {
						t.Fatal(err)
					}
					found = true
				}
			}
			if !found {
				t.Fatal("generated direct outbound missing")
			}
			var dnsOptions option.DNSOptions
			// Servers are mocked by tag; decode the actual generated DNS rules.
			var dnsDocument map[string]json.RawMessage
			if err := json.Unmarshal(document.DNS, &dnsDocument); err != nil {
				t.Fatal(err)
			}
			delete(dnsDocument, "servers")
			rawDNS, err := json.Marshal(dnsDocument)
			if err != nil {
				t.Fatal(err)
			}
			if err := dnsOptions.UnmarshalJSONContext(context.Background(), rawDNS); err != nil {
				t.Fatal(err)
			}
			dnsOptions.DisableCache = true

			t.Run("generated", func(t *testing.T) {
				result := exerciseDialer(t, target.domain, document.Route.Resolver, dnsOptions, directOptions.DialerOptions)
				if result.vpnQueries != 0 || result.directQueries == 0 || result.destination != directAnswer {
					t.Fatalf("direct domain dial used wrong DNS: destination=%v, direct=%d, VPN=%d", result.destination, result.directQueries, result.vpnQueries)
				}
			})
			t.Run("negative-control-inherited-vpn", func(t *testing.T) {
				legacyOptions := directOptions.DialerOptions
				legacyOptions.DomainResolver = nil
				result := exerciseDialer(t, target.domain, document.Route.Resolver, dnsOptions, legacyOptions)
				if result.vpnQueries == 0 || result.directQueries != 0 || result.destination != vpnAnswer {
					t.Fatalf("control did not reproduce inherited-resolver defect: %+v", result)
				}
			})
		})
	}
}

var directAnswer = netip.MustParseAddr("192.0.2.8")
var vpnAnswer = netip.MustParseAddr("198.51.100.9")

type dialResult struct {
	destination               netip.Addr
	directQueries, vpnQueries int64
}

func exerciseDialer(t *testing.T, domain, vpnTag string, dnsOptions option.DNSOptions, options option.DialerOptions) dialResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	direct := &memoryDNS{TransportAdapter: boxDNS.NewTransportAdapter("test", "direct-dns", nil), answer: directAnswer}
	vpn := &memoryDNS{TransportAdapter: boxDNS.NewTransportAdapter("test", vpnTag, nil), answer: vpnAnswer}
	manager := &memoryDNSManager{direct: direct, vpn: vpn}
	egress := &memoryEgress{}
	ctx = service.ContextWith[adapter.DNSTransportManager](ctx, manager)
	ctx = service.ContextWith[adapter.OutboundManager](ctx, &memoryOutboundManager{egress: egress})
	ctx = service.ContextWith[adapter.NetworkManager](ctx, &memoryNetworkManager{resolver: vpnTag})
	router := boxDNS.NewRouter(ctx, log.NewNOPFactory(), dnsOptions)
	ctx = service.ContextWith[adapter.DNSRouter](ctx, router)
	if err := router.Initialize(dnsOptions.Rules); err != nil {
		t.Fatal(err)
	}
	if err := router.Start(adapter.StartStateStart); err != nil {
		t.Fatal(err)
	}
	defer router.Close()

	// First prove the actual dns.rules choose direct DNS for this domain.
	addresses, err := router.Lookup(ctx, domain, adapter.DNSQueryOptions{Strategy: C.DomainStrategyIPv4Only})
	if err != nil || len(addresses) != 1 || addresses[0] != directAnswer || direct.queries.Load() == 0 || vpn.queries.Load() != 0 {
		t.Fatalf("DNS rule baseline failed: addresses=%v err=%v direct=%d VPN=%d", addresses, err, direct.queries.Load(), vpn.queries.Load())
	}
	direct.queries.Store(0)
	vpn.queries.Store(0)

	// Force the final network operation into memory. ResolverOnDetour keeps
	// the same NewWithOptions resolver-selection branch as a direct outbound.
	options.Detour = "memory-egress"
	options.DomainStrategy = option.DomainStrategy(C.DomainStrategyIPv4Only)
	if options.DomainResolver != nil {
		copyResolver := *options.DomainResolver
		copyResolver.Strategy = option.DomainStrategy(C.DomainStrategyIPv4Only)
		options.DomainResolver = &copyResolver
	}
	resolverDialer, err := dialer.NewWithOptions(dialer.Options{
		Context: ctx, Options: options, RemoteIsDomain: true,
		ResolverOnDetour: true, DirectOutbound: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := resolverDialer.DialContext(ctx, "tcp", M.Socksaddr{Fqdn: domain, Port: 443})
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
	return dialResult{egress.destination, direct.queries.Load(), vpn.queries.Load()}
}

type memoryDNS struct {
	boxDNS.TransportAdapter
	answer  netip.Addr
	queries atomic.Int64
}

func (d *memoryDNS) Start(adapter.StartStage) error { return nil }
func (d *memoryDNS) Close() error                   { return nil }
func (d *memoryDNS) Reset()                         {}
func (d *memoryDNS) Exchange(_ context.Context, request *mDNS.Msg) (*mDNS.Msg, error) {
	d.queries.Add(1)
	response := new(mDNS.Msg)
	response.SetReply(request)
	for _, q := range request.Question {
		if q.Qtype == mDNS.TypeA {
			response.Answer = append(response.Answer, &mDNS.A{
				Hdr: mDNS.RR_Header{Name: q.Name, Rrtype: mDNS.TypeA, Class: mDNS.ClassINET, Ttl: 60}, A: net.IP(d.answer.AsSlice()),
			})
		}
	}
	return response, nil
}

type memoryDNSManager struct {
	adapter.DNSTransportManager
	direct, vpn *memoryDNS
}

func (m *memoryDNSManager) Transport(tag string) (adapter.DNSTransport, bool) {
	if tag == m.direct.Tag() {
		return m.direct, true
	}
	if tag == m.vpn.Tag() {
		return m.vpn, true
	}
	return nil, false
}
func (m *memoryDNSManager) Default() adapter.DNSTransport { return m.vpn }
func (m *memoryDNSManager) Transports() []adapter.DNSTransport {
	return []adapter.DNSTransport{m.direct, m.vpn}
}

type memoryNetworkManager struct {
	adapter.NetworkManager
	resolver string
}

func (m *memoryNetworkManager) DefaultOptions() adapter.NetworkOptions {
	return adapter.NetworkOptions{DomainResolver: m.resolver}
}

type memoryOutboundManager struct {
	adapter.OutboundManager
	egress *memoryEgress
}

func (m *memoryOutboundManager) Outbound(tag string) (adapter.Outbound, bool) {
	return m.egress, tag == "memory-egress"
}

type memoryEgress struct {
	adapter.Outbound
	destination netip.Addr
}

func (d *memoryEgress) DialContext(_ context.Context, _ string, destination M.Socksaddr) (net.Conn, error) {
	if !destination.IsIP() {
		return nil, errors.New("unexpected unresolved destination")
	}
	d.destination = destination.Addr
	client, peer := net.Pipe()
	peer.Close()
	return client, nil
}
func (d *memoryEgress) ListenPacket(context.Context, M.Socksaddr) (net.PacketConn, error) {
	return nil, errors.New("unexpected packet dial")
}

const syntheticProfile = `[Interface]
PrivateKey = AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAE=
Address = 10.77.0.2/32
DNS = 1.1.1.1
[Peer]
PublicKey = AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAI=
Endpoint = 192.0.2.1:51820
AllowedIPs = 0.0.0.0/0
`
