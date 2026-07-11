# Runtime engine

Release builds use `https://github.com/hoaxisr/amnezia-box` at tag
`v1.13.13-awg2.1`, commit
`f40548f91a14582975096d0310e3c6afd44656f8`.

The binary is built by `build-public-release.ps1` with CGO disabled and these
tags:

`with_gvisor,with_quic,with_dhcp,with_wireguard,with_utls,with_acme,with_clash_api,with_awg`

No engine binary is committed to this repository. Every GitHub Release
contains the corresponding source archive and the GPL license.
