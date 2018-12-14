package main

import (
	"encoding/json"
	"io/ioutil"
	"testing"
)

// unit tests to verify the template to file conversion

func check(e error) {
	if e != nil {
		panic(e)
	}
}

func TestProcessTmplFile(t *testing.T) {
	tmpl, err := ioutil.ReadFile("./test_data/a.tmpl.txt")
	check(err)
	wants, err := ioutil.ReadFile("./test_data/a.txt")
	check(err)
	srvData, err := ioutil.ReadFile("./test_data/a.json")
	check(err)

	srv := &service{}
	err = json.Unmarshal(srvData, srv)
	check(err)

	got := ProcessTmplFile(srv, tmpl)
	if string(got) != string(wants) {
		t.Errorf("template process failed. Got \n%s\n, wants \n%s\n", string(got), string(wants))
	}
}
