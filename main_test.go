package main

import (
	"context"
	"fmt"
	"testing"
)

func TestCreation(t *testing.T) {

	srv := NewService("testing-ok")
	srv.Desc = "testing the service generator"
	token := "0a270012c6c3cd5d8fd8e9d8946989319533c07b"

	g, err := NewGitHub()
	if err != nil {
		panic(err)
	}
	err = g.authenticate(token)
	if err != nil {
		panic(err)
	}

	// validate service
	if !srv.validate(g) {
		panic("service is invalid...")
	}

	// step: create github repo
	// it was checked in prev step (validate) if the repo name was taken
	// create an empty github repo
	repoData, err := g.createRepo(context.Background(), srv)
	if err != nil {
		panic(err)
	}

	fmt.Println(*repoData.CloneURL)
	fmt.Println(*repoData.GitURL)

	// clone repo
	repo, path, err := cloneGitRepo(srv.getTmplRepoURL(), srv.getSrvRepoURL(), srv, token)

	// update the tmpl files to service related files
	ProcessTmplFolder(srv, path)

	// add, commit and push
	addCommitPush(repo, token)
}
