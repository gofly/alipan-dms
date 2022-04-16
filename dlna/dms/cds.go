package dms

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"

	"github.com/anacrolix/log"
	"github.com/gofly/alipan-dms/dlna"
	"github.com/gofly/alipan-dms/upnp"
	"github.com/gofly/alipan-dms/upnpav"
)

type browse struct {
	ObjectID       string
	BrowseFlag     string
	Filter         string
	StartingIndex  int
	RequestedCount int
}

type contentDirectoryService struct {
	*Server
	upnp.Eventing
}

// ContentDirectory object from ObjectID.
func (s *contentDirectoryService) objectFromID(id string) (o object, err error) {
	o.Path, err = url.QueryUnescape(id)
	if err != nil {
		return
	}
	if o.Path == "0" {
		o.Path = "/"
	}
	o.Path = path.Clean(o.Path)
	if !path.IsAbs(o.Path) {
		err = fmt.Errorf("bad ObjectID %v", o.Path)
		return
	}
	o.RootObjectPath = s.RootObjectPath
	return
}

func (s *contentDirectoryService) updateIDString() string {
	return fmt.Sprintf("%d", uint32(os.Getpid()))
}

// Turns the given entry and DMS host into a UPnP object. A nil object is
// returned if the entry is not of interest.
func (s *contentDirectoryService) cdsObjectToUpnpavObject(cdsObject object, fileInfo os.FileInfo, host, userAgent string) (ret interface{}, err error) {
	entryFilePath := cdsObject.FilePath()
	// ignored, err := s.IgnorePath(entryFilePath)
	// if err != nil || ignored {
	// 	return
	// }

	obj := upnpav.Object{
		ID:         cdsObject.ID(),
		Restricted: 1,
		ParentID:   cdsObject.ParentID(),
	}
	if fileInfo.IsDir() {
		obj.Class = "object.container.storageFolder"
		obj.Title = fileInfo.Name()
		ret = upnpav.Container{Object: obj, ChildCount: 0}
		return
	}
	if !fileInfo.Mode().IsRegular() {
		s.Logger.Printf("%s ignored: non-regular file", cdsObject.FilePath())
		return
	}
	// mimeType, err := MimeTypeByPath(entryFilePath)
	// if err != nil {
	// 	return
	// }

	file, ok := fileInfo.(ContentType)
	if !ok {
		s.Logger.Printf("fileInfo not a ContentType")
		return
	}
	mimeType := mimeType(file.ContentType())
	if !mimeType.IsMedia() {
		s.Logger.Printf("%s ignored: non-media file (%s)", cdsObject.FilePath(), mimeType)
		return
	}

	obj.Class = "object.item." + mimeType.Type() + "Item"
	if obj.Title == "" {
		obj.Title = fileInfo.Name()
	}

	item := upnpav.Item{
		Object: obj,
		// Capacity: 1 for raw, 1 for icon, plus transcodes.
		Res: make([]upnpav.Resource, 0, 2),
	}
	item.Res = append(item.Res, upnpav.Resource{
		URL: (&url.URL{
			Scheme: s.WebdavURI.Scheme,
			Host:   s.WebdavURI.Host,
			Path:   entryFilePath,
		}).String(),
		ProtocolInfo: fmt.Sprintf("http-get:*:%s:%s", mimeType, dlna.ContentFeatures{
			SupportRange: true,
		}.String()),
		Size: uint64(fileInfo.Size()),
	})

	ret = item
	return
}

// Returns all the upnpav objects in a directory.
func (s *contentDirectoryService) readContainer(o object, host, userAgent string) (ret []interface{}, err error) {
	fis, err := s.webdavClient.ReadDir(o.Path)
	if err != nil {
		return
	}
	for _, fi := range fis {
		child := object{path.Join(o.Path, fi.Name()), s.RootObjectPath}
		obj, err := s.cdsObjectToUpnpavObject(child, fi, host, userAgent)
		if err != nil {
			s.Logger.Printf("error with %s: %s", child.FilePath(), err)
			continue
		}
		if obj != nil {
			ret = append(ret, obj)
		}
	}
	return
}

func didlLite(chardata string) string {
	return `<DIDL-Lite` +
		` xmlns:dc="http://purl.org/dc/elements/1.1/"` +
		` xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/"` +
		` xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/"` +
		` xmlns:dlna="urn:schemas-dlna-org:metadata-1-0/">` +
		chardata +
		`</DIDL-Lite>`
}

func (s *contentDirectoryService) Handle(action string, argsXML []byte, r *http.Request) ([][2]string, error) {
	host := r.Host
	userAgent := r.UserAgent()
	log.Printf("contentDirectoryService, action: %s, xml: %s, host: %s, userAgent: %s", action, string(argsXML), host, userAgent)
	switch action {
	case "GetSystemUpdateID":
		return [][2]string{
			{"Id", s.updateIDString()},
		}, nil
	case "GetSortCapabilities":
		return [][2]string{
			{"SortCaps", "dc:title"},
		}, nil
	case "Browse":
		var browse browse
		if err := xml.Unmarshal([]byte(argsXML), &browse); err != nil {
			return nil, err
		}
		obj, err := s.objectFromID(browse.ObjectID)
		if err != nil {
			return nil, upnp.Errorf(upnpav.NoSuchObjectErrorCode, err.Error())
		}
		log.Printf("obj: %+v, browse: %+v", obj, browse)
		switch browse.BrowseFlag {
		case "BrowseDirectChildren":
			objs, err := s.readContainer(obj, host, userAgent)
			if err != nil {
				return nil, upnp.Errorf(upnpav.NoSuchObjectErrorCode, err.Error())
			}
			totalMatches := len(objs)
			objs = objs[func() (low int) {
				low = browse.StartingIndex
				if low > len(objs) {
					low = len(objs)
				}
				return
			}():]
			if browse.RequestedCount != 0 && int(browse.RequestedCount) < len(objs) {
				objs = objs[:browse.RequestedCount]
			}
			result, err := xml.Marshal(objs)
			if err != nil {
				return nil, err
			}
			s.Logger.Println(string(result))
			return [][2]string{
				{"Result", didlLite(string(result))},
				{"NumberReturned", fmt.Sprint(len(objs))},
				{"TotalMatches", fmt.Sprint(totalMatches)},
				{"UpdateID", s.updateIDString()},
			}, nil
		// case "BrowseMetadata":
		// 	fileInfo, err := os.Stat(obj.FilePath())
		// 	if err != nil {
		// 		if os.IsNotExist(err) {
		// 			return nil, &upnp.Error{
		// 				Code: upnpav.NoSuchObjectErrorCode,
		// 				Desc: err.Error(),
		// 			}
		// 		}
		// 		return nil, err
		// 	}
		// 	upnp, err := s.cdsObjectToUpnpavObject(obj, fileInfo, host, userAgent)
		// 	if err != nil {
		// 		return nil, err
		// 	}
		// 	buf, err := xml.Marshal(upnp)
		// 	if err != nil {
		// 		return nil, err
		// 	}
		// 	return [][2]string{
		// 		{"Result", didl_lite(func() string { return string(buf) }())},
		// 		{"NumberReturned", "1"},
		// 		{"TotalMatches", "1"},
		// 		{"UpdateID", s.updateIDString()},
		// 	}, nil
		default:
			return nil, upnp.Errorf(upnp.ArgumentValueInvalidErrorCode, "unhandled browse flag: %v", browse.BrowseFlag)
		}
		// case "GetSearchCapabilities":
		// 	return [][2]string{
		// 		{"SearchCaps", ""},
		// 	}, nil
		// // Samsung Extensions
		// case "X_GetFeatureList":
		// 	// TODO: make it dependable on model
		// 	// https://github.com/1100101/minidlna/blob/ca6dbba18390ad6f8b8d7b7dbcf797dbfd95e2db/upnpsoap.c#L2153-L2199
		// 	return [][2]string{
		// 		{"FeatureList", `<Features xmlns="urn:schemas-upnp-org:av:avs" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:schemaLocation="urn:schemas-upnp-org:av:avs http://www.upnp.org/schemas/av/avs.xsd">
		// 	<Feature name="samsung.com_BASICVIEW" version="1">
		// 		<container id="0" type="object.item.audioItem"/> // "A"
		// 		<container id="0" type="object.item.videoItem"/> // "V"
		// 		<container id="0" type="object.item.imageItem"/> // "I"
		// 	</Feature>
		// </Features>`},
		// 	}, nil
		// case "X_SetBookmark":
		// 	// just ignore
		// 	return [][2]string{}, nil
		// default:
		// return nil, upnp.InvalidActionError
	}
	return nil, upnp.InvalidActionError
}

// Represents a ContentDirectory object.
type object struct {
	Path           string // The cleaned, absolute path for the object relative to the server.
	RootObjectPath string
}

// Returns the actual local filesystem path for the object.
func (o *object) FilePath() string {
	return filepath.Join(o.RootObjectPath, filepath.FromSlash(o.Path))
}

// Returns the ObjectID for the object. This is used in various ContentDirectory actions.
func (o object) ID() string {
	if !path.IsAbs(o.Path) {
		log.Panicf("Relative object path: %s", o.Path)
	}
	if len(o.Path) == 1 {
		return "0"
	}
	return url.QueryEscape(o.Path)
}

func (o *object) IsRoot() bool {
	return o.Path == "/"
}

// Returns the object's parent ObjectID. Fortunately it can be deduced from the
// ObjectID (for now).
func (o object) ParentID() string {
	if o.IsRoot() {
		return "-1"
	}
	o.Path = path.Dir(o.Path)
	return o.ID()
}
