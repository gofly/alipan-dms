package main

import (
	"net"
	"net/url"
	"time"

	"github.com/anacrolix/log"

	"github.com/gofly/alipan-dms/dlna/dms"
)

func main() {
	logger := log.Default.WithNames("main")
	ifs, _ := net.Interfaces()
	webdavURI, _ := url.Parse("http://192.168.130.1:8088")
	dmsServer := &dms.Server{
		FriendlyName:   "阿里云盘",
		Interfaces:     ifs,
		RootObjectPath: "/",
		WebdavURI:      webdavURI,
		HTTPConn: func() net.Listener {
			conn, err := net.Listen("tcp", ":8083")
			if err != nil {
				log.Fatal(err)
			}
			return conn
		}(),
		NotifyInterval: time.Second * 5,
		Logger:         logger.WithNames("dms", "server"),
	}
	if err := dmsServer.Init(); err != nil {
		log.Fatalf("error initing dms server: %v", err)
	}
	if err := dmsServer.Run(); err != nil {
		log.Fatal(err)
	}
}
