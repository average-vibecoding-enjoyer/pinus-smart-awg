# Runtime engine

Release builds use `https://github.com/hoaxisr/amnezia-box` at tag
`v1.13.13-awg2.1`, commit
`f40548f91a14582975096d0310e3c6afd44656f8`.

Before compilation, `amnezia-box-security.patch` is applied to that exact
revision. The patch only updates the Go language floor and pinned
`golang.org/x/*` modules to versions containing the current security fixes.
The build fails if the patch no longer applies cleanly.
After applying it, the builder runs `go mod tidy` and verifies the exact
resulting `go.sum` SHA-256 before compiling anything.

The binary is built by `build-public-release.ps1` with CGO disabled and these
tags:

`with_gvisor,with_quic,with_dhcp,with_wireguard,with_utls,with_acme,with_clash_api,with_awg`

No engine binary is committed to this repository. Every GitHub Release
contains the corresponding upstream source archive, the exact security patch,
its SHA-256 digest, and the GPL license.
