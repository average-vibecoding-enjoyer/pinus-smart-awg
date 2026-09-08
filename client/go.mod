module github.com/amnezia-vpn/amneziawg-windows-client

go 1.26.0

require (
	github.com/amnezia-vpn/amneziawg-go/v3 v3.1.20260828
	github.com/amnezia-vpn/amneziawg-windows v0.1.9
	github.com/lxn/walk v0.0.0-20210112085537-c389da54e794
	github.com/lxn/win v0.0.0-20210218163916-a377121e959e
	golang.org/x/crypto v0.56.0
	golang.org/x/net v0.58.0
	golang.org/x/sys v0.47.0
	golang.org/x/text v0.41.0
	golang.zx2c4.com/wintun v0.0.0-20230126152724-0fa3db229ce2
)

require (
	golang.org/x/mod v0.40.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/tools v0.49.0 // indirect
)

replace (
	github.com/amnezia-vpn/amneziawg-windows => ../core
	github.com/lxn/walk => golang.zx2c4.com/wireguard/windows v0.0.0-20210121140954-e7fc19d483bd
	github.com/lxn/win => golang.zx2c4.com/wireguard/windows v0.0.0-20210224134948-620c54ef6199
)

replace github.com/amnezia-vpn/amneziawg-go/v3 => ../third_party/amneziawg-go-v3
