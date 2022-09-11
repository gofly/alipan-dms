package dmr

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/anacrolix/log"
	"github.com/gofly/alipan-dms/dlna"
	"github.com/gofly/alipan-dms/soap"
	"github.com/gofly/alipan-dms/ssdp"
	"github.com/gofly/alipan-dms/upnp"
	"github.com/gofly/alipan-dms/version"
)

const (
	userAgentProduct  = "dmr"
	rootDeviceType    = "urn:schemas-upnp-org:device:MediaRenderer:1"
	rootDescPath      = "/rootDesc.xml"
	serviceControlURL = "/ctl"
)

var (
	serverField = fmt.Sprintf(`Linux/3.4 DLNADOC/1.50 UPnP/1.0 %s/%s`,
		userAgentProduct,
		version.DmrVersion)
	rootDeviceModelName = fmt.Sprintf("%s %s", userAgentProduct, version.DmrVersion)
)

// Exposed UPnP AV services.
var services = []*dlna.Service{
	{
		Service: upnp.Service{
			ServiceType: "urn:schemas-upnp-org:service:ConnectionManager:1",
			ServiceId:   "urn:upnp-org:serviceId:ConnectionManager",
		},
		SCPD: connectionManagerServiceDescription,
	},
	{
		Service: upnp.Service{
			ServiceType: "urn:schemas-upnp-org:service:AVTransport:1",
			ServiceId:   "urn:upnp-org:serviceId:AVTransport",
			EventSubURL: "_urn:schemas-upnp-org:service:AVTransport_event",
		},
		SCPD: avTransportServiceDescription,
	},
	{
		Service: upnp.Service{
			ServiceType: "urn:schemas-upnp-org:service:RenderingControl:1",
			ServiceId:   "urn:upnp-org:serviceId:RenderingControl",
			EventSubURL: "_urn:schemas-upnp-org:service:RenderingControl_event",
		},
		SCPD: remoteControlServiceDescription,
	},
}

type Renderer struct {
}

var startTime time.Time

// Install handlers to serve SCPD for each UPnP service.
func handleSCPDs(mux *http.ServeMux) {
	for _, s := range services {
		mux.HandleFunc(s.SCPDURL, func(serviceDesc string) http.HandlerFunc {
			return func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("content-type", `text/xml; charset="utf-8"`)
				http.ServeContent(w, r, "", startTime, bytes.NewReader([]byte(serviceDesc)))
			}
		}(s.SCPD))
	}
}

func init() {
	startTime = time.Now()
	for _, s := range services {
		s.ControlURL = serviceControlURL
	}
	for _, s := range services {
		lastInd := strings.LastIndex(s.ServiceId, ":")
		p := path.Join("/scpd", s.ServiceId[lastInd+1:])
		s.SCPDURL = p + ".xml"
	}
}

func serviceTypes() (ret []string) {
	for _, s := range services {
		ret = append(ret, s.ServiceType)
	}
	return
}

type Server struct {
	FriendlyName   string
	HTTPConn       net.Listener
	Interfaces     []net.Interface
	httpServeMux   *http.ServeMux
	rootDescXML    []byte
	rootDeviceUUID string
	// Time interval between SSPD announces
	NotifyInterval time.Duration
	closed         chan struct{}
	ssdpStopped    chan struct{}
	// The service SOAP handler keyed by service URN.
	services       map[string]dlna.UPnPService
	LogHeaders     bool
	Logger         log.Logger
	eventingLogger log.Logger
}

func (s *Server) initServices() (err error) {
	urn, err := upnp.ParseServiceType(services[0].ServiceType)
	if err != nil {
		return
	}
	urn1, err := upnp.ParseServiceType(services[1].ServiceType)
	if err != nil {
		return
	}
	urn2, err := upnp.ParseServiceType(services[2].ServiceType)
	if err != nil {
		return
	}
	s.services = map[string]dlna.UPnPService{
		urn.Type: &connectionManagerService{
			Server: s,
		},
		urn1.Type: &avTransportService{
			Server: s,
		},
		urn2.Type: &remoteControlService{
			Server: s,
		},
	}
	return
}

func (s *Server) initMux(mux *http.ServeMux) {
	mux.HandleFunc(rootDescPath, func(w http.ResponseWriter, r *http.Request) {
		log.Println(rootDescPath)
		w.Header().Set("content-type", `text/xml; charset="utf-8"`)
		w.Header().Set("content-length", fmt.Sprint(len(s.rootDescXML)))
		w.Header().Set("server", serverField)
		w.Write(s.rootDescXML)
	})
	handleSCPDs(mux)
	mux.HandleFunc(serviceControlURL, s.serviceControlHandler)
	mux.HandleFunc("/debug/pprof/", pprof.Index)
}

func (s *Server) Init() (err error) {
	s.eventingLogger = s.Logger.WithNames("eventing")
	s.eventingLogger.Levelf(log.Debug, "hello %v", "world")
	if err = s.initServices(); err != nil {
		return
	}
	s.closed = make(chan struct{})
	if s.HTTPConn == nil {
		s.HTTPConn, err = net.Listen("tcp", "")
		if err != nil {
			return
		}
	}
	if s.Interfaces == nil {
		ifs, err := net.Interfaces()
		if err != nil {
			log.Print(err)
		}
		var tmp []net.Interface
		for _, if_ := range ifs {
			if if_.Flags&net.FlagUp == 0 || if_.MTU <= 0 {
				continue
			}
			tmp = append(tmp, if_)
		}
		s.Interfaces = tmp
	}

	s.httpServeMux = http.NewServeMux()
	s.rootDeviceUUID = dlna.MakeDeviceUuid(s.FriendlyName)
	s.rootDescXML, err = xml.MarshalIndent(
		upnp.DeviceDesc{
			NSDLNA:      "urn:schemas-dlna-org:device-1-0",
			NSSEC:       "http://www.sec.co.kr/dlna",
			SpecVersion: upnp.SpecVersion{Major: 1, Minor: 0},
			Device: upnp.Device{
				DeviceType:   rootDeviceType,
				FriendlyName: s.FriendlyName,
				Manufacturer: "Matt Joiner <anacrolix@gmail.com>",
				ModelName:    rootDeviceModelName,
				UDN:          s.rootDeviceUUID,
				VendorXML: `
     <dlna:X_DLNACAP/>
     <dlna:X_DLNADOC xmlns:dlna="urn:schemas-dlna-org:device-1-0">DMR-1.50</dlna:X_DLNADOC>
     <dlna:X_DLNADOC>M-DMR-1.50</dlna:X_DLNADOC>`,
				ServiceList: func() (ss []upnp.Service) {
					for _, s := range services {
						ss = append(ss, s.Service)
					}
					return
				}(),
				PresentationURL: "/",
			},
		},
		" ", "  ")
	if err != nil {
		return
	}
	s.rootDescXML = append([]byte(`<?xml version="1.0"?>`), s.rootDescXML...)
	s.Logger.Println("HTTP srv on", s.HTTPConn.Addr())
	s.initMux(s.httpServeMux)
	s.ssdpStopped = make(chan struct{})
	return nil
}

func (s *Server) Run() (err error) {
	go func() {
		s.doSSDP()
		close(s.ssdpStopped)
	}()
	return s.serveHTTP()
}

func (s *Server) Close() (err error) {
	close(s.closed)
	err = s.HTTPConn.Close()
	<-s.ssdpStopped
	return
}

func (s *Server) httpPort() int {
	return s.HTTPConn.Addr().(*net.TCPAddr).Port
}

func (s *Server) location(ip net.IP) string {
	url := url.URL{
		Scheme: "http",
		Host: (&net.TCPAddr{
			IP:   ip,
			Port: s.httpPort(),
		}).String(),
		Path: rootDescPath,
	}
	return url.String()
}

// TODO: Document the use of this for debugging.
type mitmRespWriter struct {
	http.ResponseWriter
	loggedHeader bool
	logHeader    bool
}

func (me *mitmRespWriter) WriteHeader(code int) {
	me.doLogHeader(code)
	me.ResponseWriter.WriteHeader(code)
}

func (me *mitmRespWriter) doLogHeader(code int) {
	if !me.logHeader {
		return
	}
	fmt.Fprintln(os.Stderr, code)
	for k, v := range me.Header() {
		fmt.Fprintln(os.Stderr, k, v)
	}
	fmt.Fprintln(os.Stderr)
	me.loggedHeader = true
}

func (me *mitmRespWriter) Write(b []byte) (int, error) {
	if !me.loggedHeader {
		me.doLogHeader(200)
	}
	return me.ResponseWriter.Write(b)
}

func (me *mitmRespWriter) CloseNotify() <-chan bool {
	return me.ResponseWriter.(http.CloseNotifier).CloseNotify()
}

func (s *Server) serveHTTP() error {
	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if s.LogHeaders {
				fmt.Fprintf(os.Stderr, "%s %s\r\n", r.Method, r.RequestURI)
				r.Header.Write(os.Stderr)
				fmt.Fprintln(os.Stderr)
			}
			w.Header().Set("Ext", "")
			w.Header().Set("Server", serverField)
			s.httpServeMux.ServeHTTP(&mitmRespWriter{
				ResponseWriter: w,
				logHeader:      s.LogHeaders,
			}, r)
		}),
	}
	err := srv.Serve(s.HTTPConn)
	select {
	case <-s.closed:
		return nil
	default:
		return err
	}
}

// An interface with these flags should be valid for SSDP.
const ssdpInterfaceFlags = net.FlagUp | net.FlagMulticast

func (s *Server) doSSDP() {
	var wg sync.WaitGroup
	for _, i := range s.Interfaces {
		wg.Add(1)
		go func(i net.Interface) {
			defer wg.Done()
			s.ssdpInterface(i)
		}(i)
	}
	wg.Wait()
}

// Run SSDP server on an interface.
func (s *Server) ssdpInterface(i net.Interface) {
	logger := s.Logger.WithNames("ssdp", i.Name)
	server := ssdp.Server{
		Interface: i,
		Devices: []string{
			"urn:schemas-upnp-org:device:MediaRenderer:1",
		},
		Services: serviceTypes(),
		Location: func(ip net.IP) string {
			return s.location(ip)
		},
		Server:         serverField,
		UUID:           s.rootDeviceUUID,
		NotifyInterval: s.NotifyInterval,
	}
	if err := server.Init(); err != nil {
		if i.Flags&ssdpInterfaceFlags != ssdpInterfaceFlags {
			// Didn't expect it to work anyway.
			return
		}
		if strings.Contains(err.Error(), "listen") {
			// OSX has a lot of dud interfaces. Failure to create a socket on
			// the interface are what we're expecting if the interface is no
			// good.
			return
		}
		logger.Printf("error creating ssdp server on %s: %s", i.Name, err)
		return
	}
	defer s.Close()
	logger.Levelf(log.Info, "started SSDP on %q", i.Name)
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		if err := server.Serve(); err != nil {
			logger.Printf("%q: %q\n", i.Name, err)
			return
		}
	}()
	select {
	case <-s.closed:
		// Returning will close the server.
	case <-stopped:
	}
}

// Handle a SOAP request and return the response arguments or UPnP error.
func (s *Server) soapActionResponse(sa upnp.SoapAction, actionRequestXML []byte, r *http.Request) ([][2]string, error) {
	service, ok := s.services[sa.Type]
	if !ok {
		// TODO: What's the invalid service error?!
		return nil, upnp.Errorf(upnp.InvalidActionErrorCode, "Invalid service: %s", sa.Type)
	}
	return service.Handle(sa.Action, actionRequestXML, r)
}

func xmlMarshalOrPanic(value interface{}) []byte {
	ret, err := xml.MarshalIndent(value, "", "  ")
	if err != nil {
		log.Printf("xmlMarshalOrPanic failed to marshal %v: %s", value, err)
	}
	return ret
}

// Marshal SOAP response arguments into a response XML snippet.
func marshalSOAPResponse(sa upnp.SoapAction, args [][2]string) []byte {
	soapArgs := make([]soap.Arg, 0, len(args))
	for _, arg := range args {
		argName, value := arg[0], arg[1]
		soapArgs = append(soapArgs, soap.Arg{
			XMLName: xml.Name{Local: argName},
			Value:   value,
		})
	}
	return []byte(fmt.Sprintf(`<u:%[1]sResponse xmlns:u="%[2]s">%[3]s</u:%[1]sResponse>`, sa.Action, sa.ServiceURN.String(), xmlMarshalOrPanic(soapArgs)))
}

// Handle a service control HTTP request.
func (s *Server) serviceControlHandler(w http.ResponseWriter, r *http.Request) {
	clientIp, _, _ := net.SplitHostPort(r.RemoteAddr)
	if zoneDelimiterIdx := strings.Index(clientIp, "%"); zoneDelimiterIdx != -1 {
		// IPv6 addresses may have the form address%zone (e.g. ::1%eth0)
		clientIp = clientIp[:zoneDelimiterIdx]
	}
	soapActionString := r.Header.Get("SOAPACTION")
	soapAction, err := upnp.ParseActionHTTPHeader(soapActionString)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var env soap.Envelope
	if err := xml.NewDecoder(r.Body).Decode(&env); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// AwoX/1.1 UPnP/1.0 DLNADOC/1.50
	w.Header().Set("Content-Type", `text/xml; charset="utf-8"`)
	w.Header().Set("Ext", "")
	w.Header().Set("Server", serverField)
	soapRespXML, code := func() ([]byte, int) {
		respArgs, err := s.soapActionResponse(soapAction, env.Body.Action, r)
		if err != nil {
			upnpErr := upnp.ConvertError(err)
			return xmlMarshalOrPanic(soap.NewFault("UPnPError", upnpErr)), 500
		}
		return marshalSOAPResponse(soapAction, respArgs), 200
	}()
	bodyStr := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8" standalone="yes"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body>%s</s:Body></s:Envelope>`, soapRespXML)
	w.WriteHeader(code)
	if _, err := w.Write([]byte(bodyStr)); err != nil {
		log.Print(err)
	}
}
