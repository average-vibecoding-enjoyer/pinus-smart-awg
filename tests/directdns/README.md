# Direct DNS runtime regression

This Windows-only test consumes the actual client `smart.BuildConfig` output and
the checked-out engine's real resolver-selection code, resolve dialer, DNS router,
DNS rules and DNS client. Fake DNS transports return different TEST-NET addresses
for direct and VPN queries. A test-only outbound detour ends each connection in
`net.Pipe`, so no OS socket, TUN, VPN, service or network monitor is started.

The generated VK and Russian-services direct policies must produce a direct DNS
answer and zero VPN DNS queries. A negative control removes the outbound's explicit
resolver and reproduces the previous defect: the inherited default VPN resolver
overrides the otherwise matching direct DNS rule. Removing `domain_resolver` from
the client configuration makes the generated-policy assertions fail.

Run from the task workspace with the existing isolated toolchain/cache environment:

```powershell
. .\work\update-env.ps1
Set-Location (Join-Path $UpdateRepo 'tests\directdns')
& $UpdateGo test -count=1 -v .
```

`go.mod` intentionally refers to the task's isolated engine checkout through a
relative `replace`. When using a separately extracted engine source archive,
point that replace at the extracted engine source directory first.

This checks domain-based direct DNS resolution only. It does not prove that this
defect caused a particular browser's failure, nor validate live routes, firewall
filters, interface transitions or website accessibility.
