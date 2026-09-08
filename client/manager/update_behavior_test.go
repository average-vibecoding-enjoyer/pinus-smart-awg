package manager

import (
	"errors"
	"github.com/amnezia-vpn/amneziawg-windows-client/smart"
	"reflect"
	"testing"
)

func TestFallbackOrderStopsAfterSuccessWithoutRetryLoop(t *testing.T) {
	var seen []string
	err := recoverWithFallbacks("Primary", smart.RoutingSettings{FallbackProfiles: []string{"primary", "Backup", "Backup", "Last"}}, func(name string) error {
		seen = append(seen, name)
		if name == "Backup" {
			return nil
		}
		return errors.New("offline")
	})
	if err != nil || !reflect.DeepEqual(seen, []string{"Primary", "Backup"}) {
		t.Fatalf("%v %v", seen, err)
	}
}
func TestEventQueueBoundedAndKeepsNewest(t *testing.T) {
	queue := make(chan []byte, 2)
	queueLatestEvent(queue, []byte("old"))
	queueLatestEvent(queue, []byte("middle"))
	queueLatestEvent(queue, []byte("new"))
	if string(<-queue) != "middle" || string(<-queue) != "new" {
		t.Fatal("newest event lost")
	}
	queueLatestEvent(nil, []byte("safe"))
}
func TestTraceIPRejectsMalformedResponse(t *testing.T) {
	if got, err := traceIP("fl=0\nip=2001:db8::10\n"); err != nil || got != "2001:db8::10" {
		t.Fatalf("%q %v", got, err)
	}
	for _, body := range []string{"ip=not-an-ip", "HTTP 200", "ip=192.0.2.1:443"} {
		if _, err := traceIP(body); err == nil {
			t.Fatal("malformed probe accepted")
		}
	}
}
