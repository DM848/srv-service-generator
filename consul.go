package srvgen

import (
	"github.com/pkg/errors"
	"io/ioutil"
	"net/http"
)

func GetConsulKey(key string) (val []byte, err error) {
	if key == "" {
		return nil, errors.New("key can not be empty")
	}

	client := &http.Client{
		//CheckRedirect: redirectPolicyFunc,
	}

	var resp *http.Response
	var url = "http://consul-node:8500/v1/kv/" + key + "?raw=true"
	resp, err = client.Get(url)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	val, err = ioutil.ReadAll(resp.Body)
	return
}
