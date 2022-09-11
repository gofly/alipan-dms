package dmr

import (
	"net/http"

	"github.com/anacrolix/log"
	"github.com/gofly/alipan-dms/upnp"
)

type remoteControlService struct {
	*Server
	upnp.Eventing
}

func (s *remoteControlService) Handle(action string, argsXML []byte, r *http.Request) (_ [][2]string, err error) {
	log.Println(action)
	// switch action {
	// case "SetAVTransportURI":
	// 	req := &setAVTransportURIRequest{}
	// 	err = xml.Unmarshal(argsXML, req)
	// 	if err != nil {
	// 		log.Println(err)
	// 	}
	// 	log.Printf("req: %+v", req)
	// 	return
	// case "Stop":
	// 	return
	// }
	return nil, upnp.InvalidActionError
}
