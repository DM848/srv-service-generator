package srvgen

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

func TestRemoveNonLetters(t *testing.T) {
	s1 := "fkdhfkdfhsfjhdskfsdlhfk"
	s1w := removeNonLetters(s1)
	if s1 != s1w {
		t.Errorf("miss match. Got %s, wants %s", s1w, s1)
	}

	s2 := "fkdhfkdfhsfjhdskfsdlhfk"
	s2r := "fkdh4fkdf6h--sfjhd---sk2fs¤%#¤%dl/hf#k"
	s2w := removeNonLetters(s2r)
	if s2 != s2w {
		t.Errorf("miss match. Got %s, wants %s", s2w, s2)
	}

	s3 := "fkdhfkdfhTASDAGAdskfsdlhfk"
	s3r := "fkdhfkdfhT.ASD---AGAdskfsd-lhfk"
	s3w := removeNonLetters(s3r)
	if s3 != s3w {
		t.Errorf("miss match. Got %s, wants %s", s3w, s3)
	}
}
