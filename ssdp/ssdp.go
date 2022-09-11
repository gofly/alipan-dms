package ssdp

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"
	"time"

	"github.com/anacrolix/log"
	"golang.org/x/net/ipv4"
)

const (
	AddrString = "239.255.255.250:1900"
	rootDevice = "upnp:rootdevice"
	aliveNTS   = "ssdp:alive"
	byebyeNTS  = "ssdp:byebye"
)

var NetAddr *net.UDPAddr

func init() {
	var err error
	NetAddr, err = net.ResolveUDPAddr("udp4", AddrString)
	if err != nil {
		log.Printf("Could not resolve %s: %s", AddrString, err)
	}
}

type badStringError struct {
	what string
	str  string
}

func (e *badStringError) Error() string { return fmt.Sprintf("%s %q", e.what, e.str) }

func ReadRequest(b *bufio.Reader) (req *http.Request, err error) {
	tp := textproto.NewReader(b)
	var s string
	if s, err = tp.ReadLine(); err != nil {
		return nil, err
	}
	defer func() {
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
	}()

	var f []string
	// TODO a split that only allows N values?
	if f = strings.SplitN(s, " ", 3); len(f) < 3 {
		return nil, &badStringError{"malformed request line", s}
	}
	if f[1] != "*" {
		return nil, &badStringError{"bad URL request", f[1]}
	}
	req = &http.Request{
		Method: f[0],
	}
	var ok bool
	if req.ProtoMajor, req.ProtoMinor, ok = http.ParseHTTPVersion(strings.TrimSpace(f[2])); !ok {
		return nil, &badStringError{"malformed HTTP version", f[2]}
	}

	mimeHeader, err := tp.ReadMIMEHeader()
	if err != nil {
		return nil, err
	}
	req.Header = http.Header(mimeHeader)
	return
}

type Server struct {
	conn           *net.UDPConn
	Interface      net.Interface
	Server         string
	Services       []string
	Devices        []string
	Location       func(net.IP) string
	UUID           string
	NotifyInterval time.Duration
	closed         chan struct{}
	Logger         log.Logger
}

func makeConn(ifi net.Interface) (ret *net.UDPConn, err error) {
	ret, err = net.ListenMulticastUDP("udp", &ifi, NetAddr)
	if err != nil {
		return
	}
	p := ipv4.NewPacketConn(ret)
	if err := p.SetMulticastTTL(2); err != nil {
		log.Print(err)
	}
	if err := p.SetMulticastLoopback(true); err != nil {
		log.Println(err)
	}
	return
}

func (s *Server) serve() {
	for {
		size := s.Interface.MTU
		if size > 65536 {
			size = 65536
		} else if size <= 0 { // fix for windows with mtu 4gb
			size = 65536
		}
		b := make([]byte, size)
		n, addr, err := s.conn.ReadFromUDP(b)
		select {
		case <-s.closed:
			return
		default:
		}
		if err != nil {
			s.Logger.Printf("error reading from UDP socket: %s", err)
			break
		}
		go s.handle(b[:n], addr)
	}
}

func (s *Server) Init() (err error) {
	s.closed = make(chan struct{})
	s.conn, err = makeConn(s.Interface)
	return
}

func (s *Server) Close() {
	close(s.closed)
	s.sendByeBye()
	s.conn.Close()
}

func (s *Server) Serve() (err error) {
	go s.serve()
	for {
		addrs, err := s.Interface.Addrs()
		if err != nil {
			return err
		}
		for _, addr := range addrs {
			ip := func() net.IP {
				switch val := addr.(type) {
				case *net.IPNet:
					return val.IP
				case *net.IPAddr:
					return val.IP
				}
				panic(fmt.Sprint("unexpected addr type:", addr))
			}()
			if ip.IsLinkLocalUnicast() || ip.To4() == nil {
				// These addresses seem to confuse VLC. Possibly there's supposed to be a zone
				// included in the address, but I don't see one.
				continue
			}
			extraHdrs := [][2]string{
				{"Cache-Control", fmt.Sprintf("max-age=%d", 5*s.NotifyInterval/2/time.Second)},
				{"Location", s.Location(ip)},
			}
			s.notifyAll(aliveNTS, extraHdrs)
		}
		time.Sleep(s.NotifyInterval)
	}
}

func (s *Server) usnFromTarget(target string) string {
	if target == s.UUID {
		return target
	}
	return s.UUID + "::" + target
}

func (s *Server) makeNotifyMessage(target, nts string, extraHdrs [][2]string) []byte {
	lines := [...][2]string{
		{"HOST", AddrString},
		{"NT", target},
		{"NTS", nts},
		{"SERVER", s.Server},
		{"USN", s.usnFromTarget(target)},
	}
	buf := &bytes.Buffer{}
	fmt.Fprint(buf, "NOTIFY * HTTP/1.1\r\n")
	writeHdr := func(keyValue [2]string) {
		fmt.Fprintf(buf, "%s: %s\r\n", keyValue[0], keyValue[1])
	}
	for _, pair := range lines {
		writeHdr(pair)
	}
	for _, pair := range extraHdrs {
		writeHdr(pair)
	}
	fmt.Fprint(buf, "\r\n")
	return buf.Bytes()
}

func (s *Server) send(buf []byte, addr *net.UDPAddr) {
	if n, err := s.conn.WriteToUDP(buf, addr); err != nil {
		s.Logger.Printf("error writing to UDP socket: %s", err)
	} else if n != len(buf) {
		s.Logger.Printf("short write: %d/%d bytes", n, len(buf))
	}
}

func (s *Server) delayedSend(delay time.Duration, buf []byte, addr *net.UDPAddr) {
	go func() {
		select {
		case <-time.After(delay):
			s.send(buf, addr)
		case <-s.closed:
		}
	}()
}

func (s *Server) log(args ...interface{}) {
	args = append([]interface{}{s.Interface.Name + ":"}, args...)
	s.Logger.Print(args...)
}

func (s *Server) sendByeBye() {
	for _, type_ := range s.allTypes() {
		buf := s.makeNotifyMessage(type_, byebyeNTS, nil)
		s.send(buf, NetAddr)
	}
}

func (s *Server) notifyAll(nts string, extraHdrs [][2]string) {
	for _, type_ := range s.allTypes() {
		buf := s.makeNotifyMessage(type_, nts, extraHdrs)
		delay := time.Duration(rand.Int63n(int64(100 * time.Millisecond)))
		s.delayedSend(delay, buf, NetAddr)
	}
}

func (s *Server) allTypes() (ret []string) {
	for _, a := range [][]string{
		{rootDevice, s.UUID},
		s.Devices,
		s.Services,
	} {
		ret = append(ret, a...)
	}
	return
}

func (s *Server) handle(buf []byte, sender *net.UDPAddr) {
	req, err := ReadRequest(bufio.NewReader(bytes.NewReader(buf)))
	if err != nil {
		s.Logger.Println(err)
		return
	}
	if req.Method != "M-SEARCH" || req.Header.Get("Man") != `"ssdp:discover"` {
		return
	}
	var mx uint
	if req.Header.Get("Host") == AddrString {
		mxHeader := req.Header.Get("Mx")
		i, err := strconv.ParseUint(mxHeader, 0, 0)
		if err != nil {
			s.Logger.Printf("Invalid mx header %q: %s", mxHeader, err)
			return
		}
		mx = uint(i)
	} else {
		mx = 1
	}
	types := func(st string) []string {
		if st == "ssdp:all" {
			return s.allTypes()
		}
		for _, t := range s.allTypes() {
			if t == st {
				return []string{t}
			}
		}
		return nil
	}(req.Header.Get("St"))
	for _, ip := range func() (ret []net.IP) {
		addrs, err := s.Interface.Addrs()
		if err != nil {
			panic(err)
		}
		for _, addr := range addrs {
			if ip, ok := func() (net.IP, bool) {
				switch data := addr.(type) {
				case *net.IPNet:
					if data.Contains(sender.IP) {
						return data.IP, true
					}
					return nil, false
				case *net.IPAddr:
					return data.IP, true
				}
				panic(addr)
			}(); ok {
				ret = append(ret, ip)
			}
		}
		return
	}() {
		for _, type_ := range types {
			resp := s.makeResponse(ip, type_, req)
			delay := time.Duration(rand.Int63n(int64(time.Second) * int64(mx)))
			s.delayedSend(delay, resp, sender)
		}
	}
}

func (s *Server) makeResponse(ip net.IP, targ string, req *http.Request) (ret []byte) {
	resp := &http.Response{
		StatusCode: 200,
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     make(http.Header),
		Request:    req,
	}
	for _, pair := range [...][2]string{
		{"Cache-Control", fmt.Sprintf("max-age=%d", 5*s.NotifyInterval/2/time.Second)},
		{"EXT", ""},
		{"Location", s.Location(ip)},
		{"Server", s.Server},
		{"ST", targ},
		{"USN", s.usnFromTarget(targ)},
	} {
		resp.Header.Set(pair[0], pair[1])
	}
	buf := &bytes.Buffer{}
	if err := resp.Write(buf); err != nil {
		panic(err)
	}
	return buf.Bytes()
}
