package main

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-github/v20/github"
	consulapi "github.com/hashicorp/consul/api"
	"go.uber.org/zap"
	"golang.org/x/oauth2"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/config"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
	"gopkg.in/src-d/go-git.v4/plumbing/transport/http"
)

const (
	GitHubURL         = "https://github.com/"
	Organisation      = "DM848" // or owner
	ServicePrefix     = "srv-"
	ServiceTmplPrefix = "template-" + ServicePrefix

	ConsulGitHubUser        = "GITHUB_USER"
	ConsulDockerHubPassword = "DOCKER_HUB_PWD"
	ConsulGitHubAccessToken = "GITHUB_ACCESS_TOKEN"
)

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func RandStringBytes(n int) string {
	var src = rand.NewSource(time.Now().UnixNano())
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[src.Int63()%int64(len(letterBytes))]
	}
	return string(b)
}

func NewService(name string) *service {
	return &service{
		Name:      name,
		Replicas:  1,
		CreatedAt: time.Now(),
		Port:      3000,
		Lang:      "jolie",
	}
}

type service struct {
	Name      string    `json:"name"`
	Port      uint64    `json:"port"`
	Creator   string    `json:"creator"`
	CreatedAt time.Time `json:"createdAt"`
	Desc      string    `json:"desc"`
	Tags      []string  `json:"tags"`
	Replicas  uint64    `json:"replicas"`
	Lang      string    `json:"lang"` // must match a repo suffix

	path string
}

func (s *service) validate(g *GitHub) bool {
	return s.validName() && s.validLang(g)
}

func (s *service) validName() bool {
	return len(s.Name) >= 2
}

// valid Lang validates that the language exists as a suffix to one of the repositories of the organisation
func (s *service) validLang(g *GitHub) bool {
	logger, _ := zap.NewProduction()
	defer logger.Sync() // flushes buffer, if any
	sugar := logger.Sugar()

	yes, err := g.hasRepository(ServiceTmplPrefix + s.Name)
	if err != nil {
		sugar.Panic("repository check failed",
			"err", err.Error(),
		)
		return false
	}

	// TODO: this could really need some more checks..
	for _, name := range []string{"go", "golang", "jolie", "java", "csharp", "c-sharp", "cpp", "c", "js", "javascript", "node", "nodejs", "python", "pascal"} {
		if s.Lang == name {
			yes = true
			break
		}
	}

	return yes
}

func (s *service) getTmplRepoURL() string {
	return GitHubURL + Organisation + "/" + ServiceTmplPrefix + s.Lang
}

func (s *service) getSrvRepoURL() string {
	return GitHubURL + Organisation + "/" + ServicePrefix + s.Name
}

type srvCreationStep struct {
	Success bool   `json:"success"`
	Done    bool   `json:"Done"`
	Error   string `json:"error"`
}

type serviceCreationStatus struct {
	Service             *service        `json:"service"`
	Token               string          `json:"token"` // to identify this creation
	ValidatingService   srvCreationStep `json:"validating_service"`
	CreatingGitHubRepo  srvCreationStep `json:"creating_git_hub_repo"`
	BuildingDockerImage srvCreationStep `json:"building_docker_image"`
	PublishToDockerHub  srvCreationStep `json:"publish_to_docker_hub"`
	DeployingToK8s      srvCreationStep `json:"deploying_to_k8s"`
	//Done
}

func buildDockerImage() {

}
func pushDockerImage() {}

func dockerHubLogin() {

}

func ProcessTmplFolder(srv *service, path string) {
	logger, _ := zap.NewProduction()
	defer logger.Sync() // flushes buffer, if any
	sugar := logger.Sugar()

	files, err := ioutil.ReadDir(path)
	if err != nil {
		sugar.Panic("failed to open project repo",
			"path", path,
			"err", err.Error(),
		)
	}

	for _, f := range files {
		file := path + "/" + f.Name()
		if f.IsDir() {
			ProcessTmplFolder(srv, file)
			continue
		}

		var data []byte
		data, err = ioutil.ReadFile(file)
		if err != nil {
			sugar.Infow("failed to open template file",
				"path", file,
				"err", err.Error(),
			)
			continue
		}

		data = ProcessTmplFile(srv, data)
		err = ioutil.WriteFile(file, data, 0644)
		if err != nil {
			sugar.Infow("failed to write to file",
				"path", file,
				"context", "converting tmpl to real file",
				"err", err.Error(),
			)
		}
	}
}

func ProcessTmplFile(srv *service, tmpl []byte) []byte {
	content := string(tmpl)

	content = strings.Replace(content, "{{ service.name }}", srv.Name, -1)
	content = strings.Replace(content, "{{ service.name.Capitalize() }}", strings.Title(srv.Name), -1)
	content = strings.Replace(content, "{{ service.port }}", strconv.FormatUint(srv.Port, 10), -1)
	content = strings.Replace(content, "{{ service.creator }}", srv.Creator, -1)
	content = strings.Replace(content, "{{ service.createdAt }}", srv.CreatedAt.String(), -1)
	content = strings.Replace(content, "{{ service.desc }}", srv.Desc, -1)

	tags := ""
	for i := range srv.Tags {
		tags += `"` + srv.Tags[i] + `", `
	}
	if len(tags) > 2 {
		tags = tags[:len(tags)-2]
	}
	content = strings.Replace(content, "{{ service.tags }}", tags, -1)
	content = strings.Replace(content, "{{ service.replicas }}", strconv.FormatUint(srv.Replicas, 10), -1)

	return []byte(content)
}

// clone and update remote
func cloneGitRepo(templateRepoURL, url string, service *service, token string) (repo *git.Repository, path string, err error) {
	path = "./tmp/" + RandStringBytes(8) + "/srv-" + service.Name
	service.path = path
	_ = os.MkdirAll(path, 0777)
	fmt.Println(templateRepoURL)
	repo, err = git.PlainClone(path, false, &git.CloneOptions{
		// The intended use of a GitHub personal access token is in replace of your password
		// because access tokens can easily be revoked.
		// https://help.github.com/articles/creating-a-personal-access-token-for-the-command-line/
		Auth: &http.BasicAuth{
			Username: "dm848-jenkins", // yes, this can be anything except an empty string
			Password: token,
		},
		URL:      templateRepoURL,
		Progress: os.Stdout,
	})
	if err != nil {
		return
	}

	// remove remotes and set a new one
	var remotes []*git.Remote
	remotes, err = repo.Remotes()
	if err != nil {
		return
	}
	for _, remote := range remotes {
		for i := range remote.Config().URLs {
			_ = repo.DeleteRemote(remote.Config().URLs[i])
		}
		_ = repo.DeleteRemote(remote.Config().Name)
	}

	_, err = repo.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{url},
	})

	return
}

func addCommitPush(repo *git.Repository, token string) {
	w, e := repo.Worktree()
	if e != nil {
		panic(e)
	}
	h, e := w.Commit("created service files", &git.CommitOptions{
		All: true,
		Author: &object.Signature{
			Name:  "Anders Fylling",
			Email: "fyllingz@gmail.com",
			When:  time.Now(),
		},
	})
	if e != nil {
		panic(e)
	}

	fmt.Println("hash: " + h.String())

	err := repo.Push(&git.PushOptions{
		Auth: &http.BasicAuth{
			Username: "dm848-jenkins", // yes, this can be anything except an empty string
			Password: token,
		},
	})
	if err != nil {
		panic(err)
	}
}

func NewConsul() (consul *Consul, err error) {
	consul = &Consul{}
	consul.client, err = consulapi.NewClient(consulapi.DefaultConfig())
	return
}

type Consul struct {
	client *consulapi.Client
}

func (c *Consul) GetGitHubAccessToken() (token string, err error) {
	// Get a handle to the KV API
	kv := c.client.KV()
	pair, _, err := kv.Get(ConsulGitHubAccessToken, nil)
	if err != nil {
		return
	}
	token = string(pair.Value)
	if token == "" {
		err = errors.New("missing github access token in Consul")
	}

	return
}

func (c *Consul) GetDockerPassword() (password string, err error) {
	// Get a handle to the KV API
	kv := c.client.KV()
	pair, _, err := kv.Get(ConsulDockerHubPassword, nil)
	if err != nil {
		return
	}
	password = string(pair.Value)
	if password == "" {
		err = errors.New("missing github access token in Consul")
	}

	return
}

func NewGitHub() (g *GitHub, err error) {
	g = &GitHub{}
	g.client = github.NewClient(nil)
	return
}

type GitHub struct {
	client *github.Client
}

func (g *GitHub) serviceNameAvail(name string) (available bool, err error) {
	available, err = g.hasRepository(ServicePrefix + name)
	available = !available // swap it around, cause we're making sure the repo does not exist
	return
}

func (g *GitHub) hasRepository(name string) (yes bool, err error) {
	logger, _ := zap.NewProduction()
	defer logger.Sync() // flushes buffer, if any
	sugar := logger.Sugar()

	opt := &github.RepositoryListByOrgOptions{Type: "public"}
	repos, _, err := g.client.Repositories.ListByOrg(context.Background(), Organisation, opt)
	if err != nil {
		sugar.Panic("failed to fetch repositories ",
			"err", err.Error(),
		)
		return
	}

	for _, repo := range repos {
		if name == *repo.Name {
			yes = true
			break
		}
	}
	return
}

func (g *GitHub) authenticate(token string) (err error) {
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(ctx, ts)

	g.client = github.NewClient(tc)
	return
}

func (g *GitHub) createRepo(ctx context.Context, service *service) (repository *github.Repository, err error) {
	// create a new private repository named "foo"
	repo := &github.Repository{
		Name:        github.String(ServicePrefix + service.Name),
		Description: github.String(service.Desc),
		Private:     github.Bool(false),
	}
	repository, _, err = g.client.Repositories.Create(ctx, Organisation, repo)
	return
}

func cmd(cmd string) ([]byte, error) {
	return exec.Command("sh", "-c", cmd).Output()
}

func main() {

	srv := NewService("service-generator")
	srv.Desc = "Generates services with built-in service discovery and health checking based on templates"
	srv.Creator = "Anders Fylling"
	srv.Lang = "golang"
	srv.Port = 5678
	srv.Replicas = 2
	srv.Tags = []string{"platform-endpoint"}

	token := os.Getenv("GITHUB_ACCESS_TOKEN")
	docker_pwd := os.Getenv("DOCKER_PWD")
	if docker_pwd == "" || token == "" {
		consul, err := NewConsul()
		if err != nil {
			panic(err)
		}

		if docker_pwd == "" {
			docker_pwd, err = consul.GetDockerPassword()
			if err != nil {
				panic(err)
			}
			if docker_pwd == "" {
				panic("missing docker hub password for user dm848jenkins")
			}
		}

		if token == "" {
			token, err = consul.GetGitHubAccessToken()
			if err != nil {
				panic(err)
			}
			if token == "" {
				panic("missing github access token for user dm848-jenkins")
			}
		}
	}

	g, err := NewGitHub()
	if err != nil {
		panic(err)
	}

	// validate service
	if !srv.validate(g) {
		panic("service is invalid...")
	}

	err = g.authenticate(token)
	if err != nil {
		panic(err)
	}

	// step: create github repo
	// it was checked in prev step (validate) if the repo name was taken
	// create an empty github repo
	repoData, err := g.createRepo(context.Background(), srv)
	if err != nil {
		panic(err)
	}

	// clone repo
	repo, path, err := cloneGitRepo(srv.getTmplRepoURL(), *repoData.CloneURL, srv, token)
	if err != nil {
		panic("eh: " + err.Error())
	}

	// update the tmpl files to service related files
	ProcessTmplFolder(srv, path)

	// add, commit and push
	addCommitPush(repo, token)

	// TODO jenkins
	runServer()
}
