package dmr

import "testing"

func TestAria2RPC(t *testing.T) {
	ctl := NewAria2Ctl("http://10.162.128.14:6800/jsonrpc", "zlx1989")
	// gid, err := ctl.Download("http://10.162.128.14/aria2.html", "/tmp/aria2.html")
	// if err != nil {
	// 	t.Fatal(err)
	// }
	// t.Log(gid)
	status, progress, err := ctl.GetDownloadProgress("0e9ae50c7b34d1e8")
	if err != nil {
		t.Fatal(err)
	}
	t.Log(status, progress)
}
