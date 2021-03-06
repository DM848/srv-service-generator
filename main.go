package srvgen

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

	PublicServiceTag = "platform-endpoint"

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
	srv := &service{
		Name:      name,
		CreatedAt: time.Now(),
	}
	setupDefaultService(srv)
	return srv
}

func setupDefaultService(srv *service) {
	srv.Replicas = 1
	srv.Port = 8888
	srv.Lang = "jolie"
}

type service struct {
	Name      string    `json:"name"`
	Port      uint64    `json:"port"`
	Author    string    `json:"author"`
	CreatedAt time.Time `json:"createdAt"`
	Desc      string    `json:"desc"`
	Public    bool      `json:"public"`
	Tags      []string  `json:"tags"`
	Replicas  uint64    `json:"replicas"`
	Lang      string    `json:"lang"` // must match a repo suffix

	path string
}

func (s *service) validate(g *GitHub) (err error) {
	if err = s.validateName(); err != nil {
		return
	}
	if err = s.validateLang(g); err != nil {
		return
	}
	if err = s.validateServiceNameAvailable(g); err != nil {
		return
	}

	return
}

func (s *service) validateName() error {
	if len(s.Name) < 2 {
		return errors.New("service name must be at least two character")
	}

	return nil
}

// valid Lang validates that the language exists as a suffix to one of the repositories of the organisation
func (s *service) validateLang(g *GitHub) error {
	_, err := g.hasRepository(ServiceTmplPrefix + s.Lang)
	if err != nil {
		return errors.New(err.Error() + " - Must have syntax: " + ServiceTmplPrefix + "<LANGUAGE>")
	}

	// TODO
	err = errors.New("language is not supported. Remember Java => java8, go => golang, js => javascript, etcetera")
	for _, name := range []string{"golang", "jolie", "java8", "csharp", "cpp", "c", "javascript", "nodejs", "python", "pascal"} {
		if s.Lang == name {
			err = nil
			break
		}
	}

	return err
}

// valid Lang validates that the language exists as a suffix to one of the repositories of the organisation
func (s *service) validateServiceNameAvailable(g *GitHub) error {
	_, err := g.hasRepository(ServicePrefix + s.Name)
	if err == nil {
		err = errors.New("repository exists with given service name already exists")
	} else {
		err = nil
	}
	return err
}

func (s *service) getTmplRepoURL() string {
	return GitHubURL + Organisation + "/" + ServiceTmplPrefix + s.Lang
}

func (s *service) getSrvRepoURL() string {
	return GitHubURL + Organisation + "/" + ServicePrefix + s.Name
}

func buildDockerImage() {

}
func pushDockerImage() {}

func dockerHubLogin() {

}

func ProcessTmplFolder(srv *service, path string) error {
	logger, _ := zap.NewProduction()
	defer logger.Sync() // flushes buffer, if any
	sugar := logger.Sugar()

	files, err := ioutil.ReadDir(path)
	if err != nil {
		sugar.Panic("failed to open project repo",
			"path", path,
			"err", err.Error(),
		)
		return err
	}

	for _, f := range files {
		file := path + "/" + f.Name()
		if f.IsDir() {
			_ = ProcessTmplFolder(srv, file)
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

	return nil
}

func removeNonLetters(s string) string {
	return strings.Map(
		func(r rune) rune {
			if (r >= 65 && r <= 90) || (r >= 97 && r <= 122) {
				return r
			}
			return -1
		},
		s,
	)
}

func StringProcessor(content, id, val string) string {
	content = strings.Replace(content, "{{ service."+id+" }}", val, -1)
	content = strings.Replace(content, "{{ service."+id+".Capitalize() }}", strings.Title(val), -1)
	content = strings.Replace(content, "{{ service."+id+".LettersOnly() }}", removeNonLetters(val), -1)
	content = strings.Replace(content, "{{ service."+id+".Capitalize().LettersOnly() }}", removeNonLetters(strings.Title(val)), -1)
	content = strings.Replace(content, "{{ service."+id+".LettersOnly().Capitalize() }}", removeNonLetters(strings.Title(val)), -1)

	return content
}

func ProcessTmplFile(srv *service, tmpl []byte) []byte {
	content := string(tmpl)

	content = StringProcessor(content, "name", srv.Name)
	content = StringProcessor(content, "author", srv.Author)
	content = StringProcessor(content, "desc", srv.Desc)
	content = StringProcessor(content, "description", srv.Desc)

	content = strings.Replace(content, "{{ service.port }}", strconv.FormatUint(srv.Port, 10), -1)
	content = strings.Replace(content, "{{ service.createdAt }}", srv.CreatedAt.String(), -1)

	if srv.Public {
		endpoint := false
		for i := range srv.Tags {
			endpoint = srv.Tags[i] == PublicServiceTag
			if endpoint {
				break
			}
		}

		if !endpoint {
			srv.Tags = append(srv.Tags, PublicServiceTag)
		}
	}

	tags := ""
	for i := range srv.Tags {
		tags += `"` + srv.Tags[i] + `", `
	}
	if len(tags) > 2 {
		tags = tags[:len(tags)-2]
	}
	content = strings.Replace(content, "{{ service.tags }}", strings.ToLower(tags), -1)
	content = strings.Replace(content, "{{ service.replicas }}", strconv.FormatUint(srv.Replicas, 10), -1)

	return []byte(content)
}

// clone and update remote
func cloneGitRepo(templateRepoURL, url string, service *service, token string) (repo *git.Repository, path string, err error) {
	path = "./tmp/" + RandStringBytes(8) + "-srv-" + service.Name
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
		URL: templateRepoURL,
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

func addCommitPush(repo *git.Repository, auth *http.BasicAuth) error {
	w, e := repo.Worktree()
	if e != nil {
		return e
	}
	_, e = w.Commit("created service files", &git.CommitOptions{
		All: true,
		Author: &object.Signature{
			Name:  "service-generator dm848-jenkins",
			Email: "deep-name-7269@opayq.com",
			When:  time.Now(),
		},
	})
	if e != nil {
		return e
	}

	return repo.Push(&git.PushOptions{
		Auth: auth,
	})
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
	opt := &github.RepositoryListByOrgOptions{Type: "public"}
	repos, _, err := g.client.Repositories.ListByOrg(context.Background(), Organisation, opt)
	if err != nil {
		return
	}

	err = errors.New("no repository for given name exists")
	for _, repo := range repos {
		if name == *repo.Name {
			yes = true
			err = nil
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

func Setup() {
	token := os.Getenv(ConsulGitHubAccessToken)
	docker_pwd := os.Getenv(ConsulDockerHubPassword)
	if docker_pwd == "" || token == "" {
		if docker_pwd == "" {
			bytes, err := GetConsulKey(ConsulDockerHubPassword)
			if err != nil {
				panic(err)
			}
			docker_pwd = string(bytes)
			if docker_pwd == "" {
				panic("missing docker hub password for user dm848jenkins")
			}
		}

		if token == "" {
			bytes, err := GetConsulKey(ConsulGitHubAccessToken)
			if err != nil {
				panic(err)
			}
			token = string(bytes)
			if token == "" {
				panic("missing github access token for user dm848-jenkins")
			}
		}
	}

	// service creator
	d := NewDelegator(token)

	var err error
	d.github, err = NewGitHub()
	if err != nil {
		panic(err)
	}

	err = d.github.authenticate(token)
	if err != nil {
		panic(err)
	}

	runServer(d)
}
