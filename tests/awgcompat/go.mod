module pinus.test/awgcompat

go 1.26.0

require (
	github.com/amnezia-vpn/amneziawg-go v0.2.18
	github.com/amnezia-vpn/amneziawg-go/v3 v3.1.20260828
	github.com/amnezia-vpn/amneziawg-windows v0.1.9
)

require (
	golang.org/x/crypto v0.56.0 // indirect
	golang.org/x/net v0.57.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.41.0 // indirect
	golang.zx2c4.com/wintun v0.0.0-20230126152724-0fa3db229ce2 // indirect
)

replace github.com/amnezia-vpn/amneziawg-windows => ../../core

replace github.com/amnezia-vpn/amneziawg-go/v3 => ../../third_party/amneziawg-go-v3
