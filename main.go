package main

import (
	"net"
	"net/url"
	"os"
	"time"

	"github.com/anacrolix/log"

	"github.com/gofly/alipan-dms/dlna/dmr"
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

	dmrServer := &dmr.Server{
		FriendlyName:  "下载到网盘",
		Interfaces:    inters,
		Aria2RPCAddr:  "http://127.0.0.1:6800/jsonrpc",
		Aria2RPCToken: "zlx1989",
		HTTPConn: func() net.Listener {
			conn, err := net.Listen("tcp", ":8082")
			if err != nil {
				logger.Print(err)
			}
			return conn
		}(),
		NotifyInterval: time.Second * 5,
		Logger:         logger.WithNames("dmr", "server"),
	}
	if err := dmrServer.Init(); err != nil {
		logger.Printf("[FATAL] error initing dms server: %v", err)
		os.Exit(1)
	}
	go func() {
		if err := dmrServer.Run(); err != nil {
			log.Printf("[FATAL] error runing dms server: %v", err)
			os.Exit(1)
		}
	}()
	dmsServer := &dms.Server{
		FriendlyName:   "余小胖的影院",
		Interfaces:     inters,
		RootObjectPath: "/余小胖的影院",
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
