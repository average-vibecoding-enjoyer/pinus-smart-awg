# Runtime engine

Release builds use `https://github.com/hoaxisr/amnezia-box` at commit
`f40548f91a14582975096d0310e3c6afd44656f8` (originally tagged
`v1.13.13-awg2.1`). The builder fetches and verifies this exact commit without
depending on the continued existence of the upstream tag.

Before compilation, `amnezia-box-security.patch` is applied to that exact
revision. The patch updates the Go language floor and pinned `golang.org/x/*`
modules to versions containing the current security fixes. It also bounds AWG
endpoint DNS resolution, safely rejects empty answers, and prefers IPv4 before
falling back to IPv6 so Wi-Fi/cellular transitions cannot pick an unusable
address merely because it appeared first. The build fails if the patch no
longer applies cleanly.
After applying it, the builder copies the pinned AWG 3.1 sources and verifies
the exact patched `go.sum` SHA-256 before compiling anything.

The binary is built by `build-public-release.ps1` with CGO disabled and these
tags:

`with_gvisor,with_quic,with_wireguard,with_awg`

No engine binary is committed to this repository. Release builds package the
complete patched engine sources, including the pinned AWG 3.1 module, alongside
the exact security patch, its SHA-256 digest, and the GPL license.

Preview 3.3.3 also adds an opt-in AWG TCP IPv4 fallback for a failed IPv6
connect reporting network unreachable. The known hostname is resolved through
an explicitly pinned VPN DNS transport, and the IPv4 retry uses the same AWG
device. No direct outbound or post-connect payload retry is involved. See the
endpoint regression tests included in the patch and the 3.3.3 update notes.
