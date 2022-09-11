package dmr

import (
	"encoding/xml"
	"testing"
)

func TestA(t *testing.T) {
	xmlData := `<u:SetAVTransportURI xmlns:u="urn:schemas-upnp-org:service:AVTransport:1">
    <InstanceID>0</InstanceID>
    <CurrentURI>http://127.0.0.1:3500/sample-mp4-file.mp4</CurrentURI>
    <CurrentURIMetaData>&lt;DIDL-Lite xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:sec="http://www.sec.co.kr/" xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/"&gt;&lt;item restricted="false" id="0" parentID="-1"&gt;&lt;sec:CaptionInfo sec:type="srt"&gt;http://127.0.0.1:3500/.&lt;/sec:CaptionInfo&gt;&lt;sec:CaptionInfoEx sec:type="srt"&gt;http://127.0.0.1:3500/.&lt;/sec:CaptionInfoEx&gt;&lt;upnp:class&gt;object.item.videoItem.movie&lt;/upnp:class&gt;&lt;dc:title&gt;sample-mp4-file.mp4&lt;/dc:title&gt;&lt;res protocolInfo="http-get:*:video/mp4:*"&gt;http://127.0.0.1:3500/sample-mp4-file.mp4&lt;/res&gt;&lt;res protocolInfo="http-get:*:text/srt:*"&gt;http://127.0.0.1:3500/.&lt;/res&gt;&lt;/item&gt;&lt;/DIDL-Lite&gt;</CurrentURIMetaData>
</u:SetAVTransportURI>`
	req := setAVTransportURIRequest{}
	err := xml.Unmarshal([]byte(xmlData), &req)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%+v", req)
}
