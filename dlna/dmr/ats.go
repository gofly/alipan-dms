package dmr

import (
	"encoding/xml"
	"html"
	"net/http"

	"github.com/anacrolix/log"
	"github.com/gofly/alipan-dms/upnp"
)

type setAVTransportURIRequest struct {
	XMLName         xml.Name `xml:"urn:schemas-upnp-org:service:AVTransport:1 SetAVTransportURI"`
	InstanceID      int      `xml:"InstanceID"         scpd:"A_ARG_TYPE_InstanceID,ui4"`
	CurrentURI      string   `xml:"CurrentURI"         scpd:"AVTransportURI,string"`
	CurrentMetadata string   `xml:"CurrentURIMetaData" scpd:"AVTransportURIMetaData,string"`
}

type avTransportService struct {
	*Server
	upnp.Eventing
}

func (s *avTransportService) Handle(action string, argsXML []byte, r *http.Request) (_ [][2]string, err error) {
	log.Println(action, string(argsXML))
	switch action {
	case "SetAVTransportURI":
		req := &setAVTransportURIRequest{}
		err = xml.Unmarshal(argsXML, req)
		if err != nil {
			log.Println(err)
		}
		log.Println(html.UnescapeString(req.CurrentURI))
		return [][2]string{}, nil
	case "Play", "Seek", "Stop":
		return [][2]string{}, nil
	case "GetPositionInfo":
		return [][2]string{
			{"Track", "1"},
			{"TrackDuration", "0:01:40"},
			{"TrackMetaData", ""},
			{"TrackURI", ""},
			{"RelTime", "0:01:00"},
			// {"AbsTime", "NOT_IMPLEMENTED"},
			{"AbsTime", "0:01:00"},
			{"RelCount", "2147483647"},
			{"AbsCount", "2147483647"},
		}, nil
	case "GetTransportInfo":
		return [][2]string{
			{"CurrentTransportState", "PLAYING"},
			{"CurrentTransportStatus", "OK"},
			{"CurrentSpeed", "1"},
		}, nil
	}
	return nil, upnp.InvalidActionError
}
