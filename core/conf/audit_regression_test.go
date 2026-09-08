package conf

import (
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"reflect"
	"runtime"
	"sync/atomic"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Audit-only parsing reproductions use synthetic in-memory configurations.
// The writer reproduction below touches only a temporary directory and uses
// a test descriptor permitting the current user, without SYSTEM ownership.
// No test invokes production profile storage, DNS, IPC, or tunnel APIs.
func TestAuditInvalidCIDRRejectedWithoutPanic(t *testing.T) {
	syntheticKey := base64.StdEncoding.EncodeToString(make([]byte, 32))
	peerKeyBytes := make([]byte, 32)
	peerKeyBytes[0] = 1
	syntheticPeerKey := base64.StdEncoding.EncodeToString(peerKeyBytes)
	for _, prefix := range []string{"10.0.0.1/33", "10.0.0.1/-1", "2001:db8::1/129"} {
		for _, field := range []string{"Address", "AllowedIPs"} {
			t.Run(field+"_"+prefix, func(t *testing.T) {
				profile := "[Interface]\nPrivateKey = " + syntheticKey + "\n"
				if field == "Address" {
					profile += "Address = " + prefix + "\n"
				} else {
					profile += "[Peer]\nPublicKey = " + syntheticPeerKey + "\nAllowedIPs = " + prefix + "\n"
				}
				defer func() {
					if panicValue := recover(); panicValue != nil {
						t.Errorf("invalid %s %q must return a validation error, not panic: %v", field, prefix, panicValue)
					}
				}()
				config, err := FromWgQuick(profile, "audit")
				if err == nil {
					t.Errorf("invalid %s %q was accepted: config is nil=%v", field, prefix, config == nil)
				}
			})
		}
	}
}

func TestAuditAllowedIPsDeduplicatedPerPeer(t *testing.T) {
	cidr := func(s string) IPCidr {
		ip, network, err := net.ParseCIDR(s)
		if err != nil {
			t.Fatal(err)
		}
		bits, _ := network.Mask.Size()
		if v4 := ip.To4(); v4 != nil {
			ip = v4
		}
		return IPCidr{IP: ip, Cidr: uint8(bits)}
	}
	c := Config{Peers: []Peer{
		{AllowedIPs: []IPCidr{cidr("10.0.0.0/24"), cidr("10.0.0.0/24"), cidr("10.1.0.0/24")}},
		{AllowedIPs: []IPCidr{cidr("2001:db8::/32"), cidr("2001:db8::/32")}},
	}}
	c.DeduplicateNetworkEntries()
	expected := [][]string{{"10.0.0.0/24", "10.1.0.0/24"}, {"2001:db8::/32"}}
	for i := range c.Peers {
		t.Run(fmt.Sprintf("peer_%d", i), func(t *testing.T) {
			var actual []string
			for _, prefix := range c.Peers[i].AllowedIPs {
				actual = append(actual, prefix.String())
			}
			if !reflect.DeepEqual(actual, expected[i]) {
				t.Errorf("duplicate AllowedIPs remain after normalization: got %v, want %v", actual, expected[i])
			}
		})
	}
}

func TestAuditWriterPreservesRelativeDestinationName(t *testing.T) {
	// Do not use t.Parallel: the real writer uses a package-global descriptor
	// and resolves its random source filename against the process directory.
	scratch := t.TempDir()
	t.Chdir(scratch)

	tokenUser, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatalf("read current user for scratch-only ACL: %v", err)
	}
	descriptor, err := windows.SecurityDescriptorFromString("D:(A;;FA;;;" + tokenUser.User.Sid.String() + ")")
	if err != nil {
		t.Fatalf("create scratch-only descriptor: %v", err)
	}
	previousDescriptor := atomic.LoadPointer(&encryptedFileSd)
	atomic.StorePointer(&encryptedFileSd, unsafe.Pointer(descriptor))
	defer func() {
		atomic.StorePointer(&encryptedFileSd, previousDescriptor)
		runtime.KeepAlive(descriptor)
	}()

	// All names are relative leaves. Even if the API truncates the target
	// to "abcd", it and the random source file remain under scratch.
	const destination = "abcdefgh"
	const contents = "synthetic Pinus audit fixture; no profile data\n"
	if err := writeLockedDownFile(destination, false, []byte(contents)); err != nil {
		t.Fatalf("actual writer failed inside isolated scratch directory: %v", err)
	}
	data, err := os.ReadFile(destination)
	if err != nil {
		entries, listErr := os.ReadDir(".")
		var names []string
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Fatalf("requested destination missing after successful write: %v; scratch names=%v; listing error=%v", err, names, listErr)
	}
	if string(data) != contents {
		t.Errorf("writer changed synthetic file contents: got %q, want %q", string(data), contents)
	}
}
