package main

import (
	"net"
	"net/url"
	"os"
	"time"

	"github.com/anacrolix/log"

	"github.com/gofly/alipan-dms/dlna/dms"
)

func main() {
	logger := log.Default.WithNames("main")
	webdavURI, err := url.Parse(os.Getenv("ALIYUNDRIVE_WEBDAV"))
	if err != nil {
		logger.Printf("[FATAL] env ALIYUNDRIVE_WEBDAV invalid")
		os.Exit(1)
	}
	inter := os.Getenv("INTERFACE")
	inters := make([]net.Interface, 0)
	ifs, _ := net.Interfaces()
	for _, i := range ifs {
		if inter == "" || i.Name == inter {
			inters = append(inters, i)
		}
	}
	dmsServer := &dms.Server{
		FriendlyName:   "阿里云盘",
		Interfaces:     inters,
		RootObjectPath: "/",
		WebdavURI:      webdavURI,
		HTTPConn: func() net.Listener {
			conn, err := net.Listen("tcp", ":8083")
			if err != nil {
				logger.Print(err)
			}
			return conn
		}(),
		NotifyInterval: time.Second * 5,
		Logger:         logger.WithNames("dms", "server"),
	}
	if err := dmsServer.Init(); err != nil {
		logger.Printf("[FATAL] error initing dms server: %v", err)
		os.Exit(1)
	}
	if err := dmsServer.Run(); err != nil {
		log.Printf("[FATAL] error runing dms server: %v", err)
		os.Exit(1)
	}
}
