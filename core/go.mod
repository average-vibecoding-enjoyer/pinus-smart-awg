module github.com/amnezia-vpn/amneziawg-windows

go 1.26.0

require (
	github.com/amnezia-vpn/amneziawg-go/v3 v3.1.20260828
	golang.org/x/crypto v0.56.0
	golang.org/x/sys v0.47.0
	golang.org/x/text v0.42.0
)

require (
	golang.org/x/net v0.57.0 // indirect
	golang.zx2c4.com/wintun v0.0.0-20230126152724-0fa3db229ce2 // indirect
)

replace github.com/amnezia-vpn/amneziawg-go/v3 => ../third_party/amneziawg-go-v3
