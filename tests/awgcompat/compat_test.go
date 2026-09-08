// These tests never create a socket, OS adapter, service, route or DNS request.
package awgcompat

import (
 "bytes"
 "encoding/binary"
 "fmt"
 "io"
 "net"
 "net/netip"
 "os"
 "strings"
 "sync"
 "testing"
 "time"
 "github.com/amnezia-vpn/amneziawg-windows/conf"
 oldconn "github.com/amnezia-vpn/amneziawg-go/conn"
 olddev "github.com/amnezia-vpn/amneziawg-go/device"
 oldtun "github.com/amnezia-vpn/amneziawg-go/tun"
 newconn "github.com/amnezia-vpn/amneziawg-go/v3/conn"
 newdev "github.com/amnezia-vpn/amneziawg-go/v3/device"
 newtun "github.com/amnezia-vpn/amneziawg-go/v3/tun"
)

type memoryEndpoint string
func (e memoryEndpoint) ClearSrc() {}
func (e memoryEndpoint) SrcToString() string { return "" }
func (e memoryEndpoint) DstToString() string { return string(e) }
func (e memoryEndpoint) DstToBytes() []byte { return []byte(e) }
func (e memoryEndpoint) SrcIP() netip.Addr { return netip.Addr{} }
func (e memoryEndpoint) DstIP() netip.Addr { return netip.MustParseAddrPort(string(e)).Addr() }

type wirePacket struct { data []byte; from string }
type memoryBind struct {
 sync.Mutex
 in, out chan wirePacket
 closed chan struct{}
 local string
}
func (b *memoryBind) open(port uint16) { b.Lock(); defer b.Unlock(); b.closed=make(chan struct{}); b.local=fmt.Sprintf("127.0.0.1:%d",port) }
func (b *memoryBind) Close() error { b.Lock(); defer b.Unlock(); if b.closed != nil { select { case <-b.closed: default:close(b.closed) } }; return nil }
func (b *memoryBind) SetMark(uint32) error { return nil }
func (b *memoryBind) BatchSize() int { return 1 }
func (b *memoryBind) send(bufs [][]byte) error {
 b.Lock(); closed, local := b.closed, b.local; b.Unlock()
 for _, buf := range bufs { select { case b.out <- wirePacket{append([]byte(nil),buf...),local}: case <-closed: return net.ErrClosed } }; return nil
}
func (b *memoryBind) receive(bufs [][]byte, sizes []int) (string,error) {
 b.Lock(); closed:=b.closed; b.Unlock()
 select {case p:=<-b.in: sizes[0]=copy(bufs[0],p.data); return p.from,nil; case <-closed:return "",net.ErrClosed}
}
type memoryTun struct { in,out chan []byte; closed chan struct{}; once sync.Once }
func makeTun() *memoryTun { return &memoryTun{in:make(chan []byte,32),out:make(chan []byte,32),closed:make(chan struct{})} }
func (t *memoryTun) File() *os.File { return nil }
func (t *memoryTun) MTU() (int,error) { return 1280,nil }
func (t *memoryTun) Name() (string,error) { return "memory-only",nil }
func (t *memoryTun) BatchSize() int { return 1 }
func (t *memoryTun) Read(bufs [][]byte,sizes []int,offset int)(int,error) { select { case p:=<-t.in:sizes[0]=copy(bufs[0][offset:],p);return 1,nil;case <-t.closed:return 0,io.EOF } }
func (t *memoryTun) Write(bufs [][]byte,offset int)(int,error) { for _,b:= range bufs { select {case t.out<-append([]byte(nil),b[offset:]...):case <-t.closed:return 0,io.EOF} };return len(bufs),nil }
type runningDevice interface { IpcSet(string) error; IpcGet() (string,error); Up() error; Close() }

type oldBind struct { *memoryBind }
func (b *oldBind) Open(port uint16)([]oldconn.ReceiveFunc,uint16,error) { b.open(port); return []oldconn.ReceiveFunc{func(bufs [][]byte,sizes []int,eps []oldconn.Endpoint)(int,error) { from,err:=b.receive(bufs,sizes);if err!=nil{return 0,err};eps[0]=memoryEndpoint(from);return 1,nil } },port,nil }
func (b *oldBind) Send(bufs [][]byte,ep oldconn.Endpoint) error { return b.send(bufs) }
func (b *oldBind) ParseEndpoint(s string)(oldconn.Endpoint,error) { return memoryEndpoint(s),nil }
type oldTun struct { *memoryTun; events chan oldtun.Event }
func (t *oldTun) Events() <-chan oldtun.Event { return t.events }
func (t *oldTun) Close() error { t.once.Do(func(){close(t.closed);close(t.events)});return nil }
func makeoldDevice(tun *memoryTun,bind *memoryBind) runningDevice { return olddev.NewDevice(&oldTun{tun,make(chan oldtun.Event)},&oldBind{bind},&olddev.Logger{Verbosef:func(f string,a ...any){if !strings.HasPrefix(f,"Routine:") {fmt.Printf("AWG: "+f+"\n",a...)}},Errorf:func(f string,a ...any){fmt.Printf("AWG ERROR: "+f+"\n",a...)}}) }

type newBind struct { *memoryBind }
func (b *newBind) Open(port uint16)([]newconn.ReceiveFunc,uint16,error) { b.open(port); return []newconn.ReceiveFunc{func(bufs [][]byte,sizes []int,eps []newconn.Endpoint)(int,error) { from,err:=b.receive(bufs,sizes);if err!=nil{return 0,err};eps[0]=memoryEndpoint(from);return 1,nil } },port,nil }
func (b *newBind) Send(bufs [][]byte,ep newconn.Endpoint) error { return b.send(bufs) }
func (b *newBind) ParseEndpoint(s string)(newconn.Endpoint,error) { return memoryEndpoint(s),nil }
type newTun struct { *memoryTun; events chan newtun.Event }
func (t *newTun) Events() <-chan newtun.Event { return t.events }
func (t *newTun) Close() error { t.once.Do(func(){close(t.closed);close(t.events)});return nil }
func makenewDevice(tun *memoryTun,bind *memoryBind) runningDevice { return newdev.NewDevice(&newTun{tun,make(chan newtun.Event)},&newBind{bind},&newdev.Logger{Verbosef:func(f string,a ...any){if !strings.HasPrefix(f,"Routine:") {fmt.Printf("AWG: "+f+"\n",a...)}},Errorf:func(f string,a ...any){fmt.Printf("AWG ERROR: "+f+"\n",a...)}}) }

func packet(src,dst byte, payload string) []byte {
 b:=make([]byte,20+len(payload));b[0]=0x45;binary.BigEndian.PutUint16(b[2:4],uint16(len(b)));b[8]=64;b[9]=253
 copy(b[12:16],[]byte{10,77,0,src});copy(b[16:20],[]byte{10,77,0,dst});copy(b[20:],payload)
 var sum uint32;for i:=0;i<20;i+=2 {sum+=uint32(binary.BigEndian.Uint16(b[i:i+2]))}; for sum>>16 != 0 {sum=(sum&65535)+(sum>>16)};binary.BigEndian.PutUint16(b[10:12], ^uint16(sum));return b
}
func TestInMemoryWireCompatibility(t *testing.T) {
 for _,tc:=range []struct{name,params string;oldServer bool}{
  {"WireGuard", "",true},
  {"AWG1", "Jc = 2\nJmin = 16\nJmax = 32\nS1 = 20\nS2 = 30\nH1 = 101\nH2 = 202\nH3 = 303\nH4 = 404\n",true},
  {"AWG2", "S1 = 20\nS2 = 30\nS3 = 40\nS4 = 12\nH1 = 100-199\nH2 = 200-299\nH3 = 300-399\nH4 = 400-499\nI1 = <b 0x01020304><r 16>\n",true},
  {"AWG31", "S1 = 16\nS2 = 16\nS3 = 16\nS4 = 16\nHeaderProtectionKey = AgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgI=\nContentPaddingAddition = 0-32\nRekeyAfterTime = 110-130\nRekeyTimeout = 4-6\nRejectAfterTime = 170-190\nKeepaliveTimeout = 8-12\nMaxHandshakeAttempts = 15-20\nRandomTrailers = on\nDisableCookies = on\n",false},
 } {t.Run(tc.name,func(t *testing.T){
  aTun,bTun:=makeTun(),makeTun();aWire,bWire:=make(chan wirePacket,512),make(chan wirePacket,512)
  a:=makenewDevice(aTun,&memoryBind{in:aWire,out:bWire})
  var b runningDevice;if tc.oldServer {b=makeoldDevice(bTun,&memoryBind{in:bWire,out:aWire})}else{b=makenewDevice(bTun,&memoryBind{in:bWire,out:aWire})}
  defer a.Close();defer b.Close()
  ak,_:=conf.NewPrivateKey();bk,_:=conf.NewPrivateKey()
  for n,side:=range []struct{d runningDevice;k,p *conf.Key}{{a,ak,bk.Public()},{b,bk,ak.Public()}} {
   text:=fmt.Sprintf("[Interface]\nPrivateKey = %s\nListenPort = %d\nAddress = 10.77.0.%d/24\nMTU = 1280\n%s[Peer]\nPublicKey = %s\nAllowedIPs = 10.77.0.0/24\nEndpoint = 127.0.0.1:%d\n",side.k.String(),40001+n,n+1,tc.params,side.p.String(),40002-n)
   if !tc.oldServer && n == 0 {text += "PersistentKeepalive = 20-30\n"}
   cfg,err:=conf.FromWgQuick(text,"Synthetic");if err!=nil{t.Fatal(err)};uapi,err:=cfg.ToUAPI();if err!=nil{t.Fatal(err)}
   if err=side.d.IpcSet(uapi);err!=nil{t.Fatal(err)};if err=side.d.Up();err!=nil{t.Fatal(err)}
  }
  exchange:=func(send,receive *memoryTun, want []byte) { t.Helper();send.in<-want;select {case got:=<-receive.out: if !bytes.Equal(got,want){t.Fatalf("decrypted payload changed: want %d bytes, got %d",len(want),len(got))};case <-time.After(5*time.Second): t.Fatalf("in-memory handshake/data timed out (source=%d destination=%d payload=%s)",want[15],want[19],string(want[20:]))} }
  for n:=0;n<12;n++ {exchange(aTun,bTun,packet(1,2,fmt.Sprintf("client payload %d",n)));exchange(bTun,aTun,packet(2,1,strings.Repeat("server payload",n+1)))}
  for _,side:=range []runningDevice{a,b}{status,err:=side.IpcGet();if err!=nil{t.Fatal(err)};if strings.Contains(status,"last_handshake_time_sec=0")||!strings.Contains(status,"last_handshake_time_sec="){t.Fatal("missing successful handshake")}}
 }) }
}
