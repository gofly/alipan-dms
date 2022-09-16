package dmr

import (
	"encoding/xml"
	"fmt"
	"html"
	"net/http"

	"github.com/anacrolix/log"
	"github.com/gofly/alipan-dms/upnp"
)

type Aria2Service interface {
	Download(uri, out string) (string, error)
	GetDownloadProgress(string) (string, int, error)
	// GetTransportInfo(string) (bool, error)
}

type setAVTransportURIRequest struct {
	XMLName         xml.Name `xml:"urn:schemas-upnp-org:service:AVTransport:1 SetAVTransportURI"`
	InstanceID      int      `xml:"InstanceID"         scpd:"A_ARG_TYPE_InstanceID,ui4"`
	CurrentURI      string   `xml:"CurrentURI"         scpd:"AVTransportURI,string"`
	CurrentMetadata string   `xml:"CurrentURIMetaData" scpd:"AVTransportURIMetaData,string"`
}

type avTransportService struct {
	*Server
	upnp.Eventing
	Aria2Service
	gid      string
	status   string
	progress int
}

func (s *avTransportService) Handle(action string, argsXML []byte, r *http.Request) (_ [][2]string, err error) {
	switch action {
	case "SetAVTransportURI":
		req := &setAVTransportURIRequest{}
		err = xml.Unmarshal(argsXML, req)
		if err != nil {
			log.Println(err)
		} else {
			s.gid, err = s.Download(html.UnescapeString(req.CurrentURI), "video")
			if err != nil {
				log.Println(err)
			}
		}
		return [][2]string{}, err
	case "Play", "Seek", "Stop":
		return [][2]string{}, nil
	case "GetPositionInfo":
		s.status, s.progress, err = s.GetDownloadProgress(s.gid)
		if err != nil {
			return
		}
		t := fmt.Sprintf("0:%02d:%02d", s.progress/60, s.progress%60)
		return [][2]string{
			{"Track", "1"},
			{"TrackDuration", "0:01:40"},
			{"TrackMetaData", ""},
			{"TrackURI", ""},
			{"RelTime", t},
			{"AbsTime", t},
			{"RelCount", "2147483647"},
			{"AbsCount", "2147483647"},
		}, nil
	case "GetTransportInfo":
		state := "PLAYING"
		switch s.status {
		case "paused":
			state = "PAUSED"
		case "complete":
			state = "STOPPED"
		}
		return [][2]string{
			{"CurrentTransportState", state},
			{"CurrentTransportStatus", "OK"},
			{"CurrentSpeed", "1"},
		}, nil
	}
	return nil, upnp.InvalidActionError
}
