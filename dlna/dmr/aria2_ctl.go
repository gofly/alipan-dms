package dmr

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

type jsonrpcMessage struct {
	Version string      `json:"jsonrpc,omitempty"`
	ID      int64       `json:"id,omitempty"`
	Method  string      `json:"method,omitempty"`
	Params  interface{} `json:"params,omitempty"`
}

type Aria2Ctl struct {
	rpcAddr  string
	rpcToken string
	client   *http.Client
}

func NewAria2Ctl(addr, token string) *Aria2Ctl {
	return &Aria2Ctl{
		rpcAddr:  addr,
		rpcToken: token,
		client: &http.Client{
			Timeout: time.Second,
			Transport: &http.Transport{
				MaxIdleConns:          10,
				IdleConnTimeout:       90 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
			},
		},
	}
}

func (c *Aria2Ctl) Call(method string, params []interface{}, result interface{}) error {
	params = append([]interface{}{"token:" + c.rpcToken}, params...)
	data, err := json.Marshal(jsonrpcMessage{
		Version: "2.0",
		ID:      time.Now().UnixNano(),
		Method:  method,
		Params:  params,
	})
	if err != nil {
		return err
	}
	resp, err := c.client.Post(c.rpcAddr, "application/json", bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("http %s", resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(&struct {
		Result interface{}
	}{
		Result: result,
	})
}

func (c *Aria2Ctl) Download(uri, out string) (gid string, err error) {
	err = c.Call("aria2.addUri", []interface{}{
		[]string{uri}, map[string]string{"out": out},
	}, &gid)
	return
}

func (c *Aria2Ctl) GetDownloadProgress(gid string) (status string, v int, err error) {
	result := struct {
		Gid             string `json:"gid"`
		CompletedLength string `json:"completedLength"`
		TotalLength     string `json:"totalLength"`
		Status          string `json:"status"`
	}{}
	err = c.Call("aria2.tellStatus", []interface{}{gid}, &result)
	if err != nil {
		return "error", 0, err
	}
	completed, _ := strconv.Atoi(result.CompletedLength)
	total, _ := strconv.Atoi(result.TotalLength)
	if total > 0 {
		v = completed * 100 / total
	}
	status = result.Status
	return
}

// func (c *Aria2Ctl) GetTransportInfo(gid string) (bool, error) {

// }
