module pinus.test/directdns

go 1.26.0

require (
	github.com/amnezia-vpn/amneziawg-windows v0.1.9
	github.com/amnezia-vpn/amneziawg-windows-client v0.0.0
	github.com/miekg/dns v1.1.72
	github.com/sagernet/sing v0.8.10
	github.com/sagernet/sing-box v0.0.0
)

require (
	github.com/database64128/tfo-go/v2 v2.3.2 // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/google/btree v1.1.3 // indirect
	github.com/logrusorgru/aurora v2.0.3+incompatible // indirect
	github.com/sagernet/fswatch v0.1.2 // indirect
	github.com/sagernet/sing-tun v0.8.10 // indirect
	github.com/vishvananda/netns v0.0.5 // indirect
	go4.org/netipx v0.0.0-20231129151722-fdeea329fbba // indirect
	golang.org/x/crypto v0.56.0 // indirect
	golang.org/x/exp v0.0.0-20251219203646-944ab1f22d93 // indirect
	golang.org/x/mod v0.40.0 // indirect
	golang.org/x/net v0.58.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.41.0 // indirect
)

replace github.com/amnezia-vpn/amneziawg-windows-client => ../../client

replace github.com/amnezia-vpn/amneziawg-windows => ../../core

replace github.com/amnezia-vpn/amneziawg-go/v3 => ../../third_party/amneziawg-go-v3

replace github.com/sagernet/sing-box => ../../../../work/update-runtime/engine-source

replace github.com/lxn/walk => golang.zx2c4.com/wireguard/windows v0.0.0-20210121140954-e7fc19d483bd

replace github.com/lxn/win => golang.zx2c4.com/wireguard/windows v0.0.0-20210224134948-620c54ef6199
