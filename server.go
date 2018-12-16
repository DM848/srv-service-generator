package srvgen

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/julienschmidt/httprouter"
	httpgit "gopkg.in/src-d/go-git.v4/plumbing/transport/http"
)

type srvCreationStep struct {
	Success bool   `json:"success"`
	Done    bool   `json:"done"`
	Message string `json:"message"`
}

type serviceCreationStatus struct {
	sync.RWMutex
	Service            *service        `json:"service"`
	Token              string          `json:"service_progress_token"` // to identify this creation
	ValidatingService  srvCreationStep `json:"validating_service"`
	CreatingGitHubRepo srvCreationStep `json:"creating_git_hub_repo"`
	//BuildingDockerImage srvCreationStep `json:"building_docker_image"`
	//PublishToDockerHub  srvCreationStep `json:"publish_to_docker_hub"`
	//DeployingToK8s      srvCreationStep `json:"deploying_to_k8s"`
	BindingJenkins srvCreationStep `json:"bindingJenkins"`
	Done           bool            `json:"done"`
	GitHubURL      string          `json:"github_url,omitempty"`
	GatewayURI     string          `json:"gateway_uri,omitempty"`
}

const (
	Success = "success"
	Fail    = "fail"
	Error   = "error"

	_ = iota
	ErrCodeFullQue
	ErrCodeMissingAuthor
	ErrCodeMissingSrvName
)

type JSend struct {
	Status    string      `json:"status"`
	Data      interface{} `json:"data,omitempty"`
	Message   string      `json:"message,omitempty"`
	ErrorCode int         `json:"error_code,omitempty"`
}

func (j *JSend) write(w http.ResponseWriter) {
	if j.Data == nil && j.Message != "" {
		j.Status = Error
	}

	body, err := json.Marshal(j)
	if err != nil {
		body = []byte(`{"status":"error","message":"unable to correctly parse response on server"}`)
	}

	_, _ = w.Write(body)
}

func NewDelegator(token string) *Delegator {
	d := &Delegator{
		jobs: make(chan *serviceCreationStatus, 20),
		gitAuth: &httpgit.BasicAuth{
			Username: "dm848-jenkins", // yes, this can be anything except an empty string
			Password: token,
		},
		token: token,
	}
	go d.worker()
	return d
}

type Delegator struct {
	sync.RWMutex
	jobs chan *serviceCreationStatus

	records []*serviceCreationStatus

	token  string
	github *GitHub

	gitAuth *httpgit.BasicAuth
}

func (d *Delegator) shutdown() {
	discordedJobs := "Shutting down. The following service creations were discarded:\n"

	i := 1
	for job := range d.jobs {
		discordedJobs += fmt.Sprintf("%d: %s\n", i, job.Service.Name)
		i++
	}
	close(d.jobs)
	fmt.Println(fmt.Errorf(discordedJobs))
}

func (d *Delegator) worker() {
	var open bool
	var progress *serviceCreationStatus
	for {
		if progress != nil && progress.Service.path != "" {
			// TODO: check for errors and clean up the github repo and cloned directory
			err := os.RemoveAll(progress.Service.path)
			if err != nil {
				fmt.Println(err.Error())
			}

			_, err = d.github.client.Repositories.Delete(context.Background(), Organisation, ServicePrefix+progress.Service.Name)
			if err != nil {
				fmt.Println(err.Error())
			}
		}
		select {
		case progress, open = <-d.jobs:
			if !open {
				return // shutting down
			}
		}

		d.Lock()
		d.records = append(d.records, progress)
		d.Unlock()

		// validate service
		progress.Lock()
		progress.ValidatingService.Success = true
		progress.ValidatingService.Done = true
		if err := progress.Service.validate(d.github); err != nil {
			progress.ValidatingService.Message = err.Error()
			progress.ValidatingService.Success = false
			progress.Unlock()
			continue
		}
		progress.Unlock()

		// step: create github repo for the service
		repoData, err := d.github.createRepo(context.Background(), progress.Service)
		progress.Lock()
		if err != nil {
			progress.CreatingGitHubRepo.Done = true
			progress.CreatingGitHubRepo.Message = err.Error()
			progress.Unlock()
			continue
		}
		progress.Unlock()

		// clone repo
		repo, path, err := cloneGitRepo(progress.Service.getTmplRepoURL(), *repoData.CloneURL, progress.Service, d.token)
		progress.Lock()
		if err != nil {
			progress.CreatingGitHubRepo.Done = true
			progress.CreatingGitHubRepo.Message = err.Error()
			progress.Unlock()
			continue
		}
		progress.Unlock()

		// update the tmpl files to service related files
		err = ProcessTmplFolder(progress.Service, path)
		progress.Lock()
		if err != nil {
			progress.CreatingGitHubRepo.Done = true
			progress.CreatingGitHubRepo.Message = err.Error()
			progress.Unlock()
			continue
		}
		progress.Unlock()

		// add, commit and push
		err = addCommitPush(repo, d.gitAuth)
		progress.Lock()
		progress.CreatingGitHubRepo.Done = true
		progress.CreatingGitHubRepo.Success = true
		if err != nil {
			progress.CreatingGitHubRepo.Message = err.Error()
			progress.CreatingGitHubRepo.Success = false
		}

		progress.BindingJenkins.Success = true
		progress.BindingJenkins.Message = "jenkins is not yet supported"
		progress.Done = true

		// add github url if all way ok
		if progress.CreatingGitHubRepo.Success == true {
			progress.GitHubURL = progress.Service.getSrvRepoURL()

			if progress.Service.Public {
				progress.GatewayURI = "/api/" + progress.Service.Name
			}
		}

		progress.Unlock()

		// TODO jenkins
	}
}

func (d *Delegator) createService(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	// make sure there's included name
	var err error
	var body []byte
	jsend := &JSend{}
	body, err = ioutil.ReadAll(r.Body)
	if err != nil {
		log.Fatal(err)
	}
	defer r.Body.Close()

	srv := &service{
		CreatedAt: time.Now(),
	}
	err = json.Unmarshal(body, srv)
	if err != nil {
		log.Fatal(err)
	}

	if len(srv.Name) < 2 {
		jsend.Message = "missing service name. Must be at least 2 character long"
		jsend.ErrorCode = ErrCodeMissingSrvName
		jsend.write(w)
		return
	}

	if srv.Author == "" {
		jsend.Message = "missing name of creator / author. Can not be empty"
		jsend.ErrorCode = ErrCodeMissingAuthor
		jsend.write(w)
		return
	}

	setupDefaultService(srv)
	_ = json.Unmarshal(body, srv)

	progress := &serviceCreationStatus{
		Service: srv,
		Token:   RandStringBytes(11),
	}

	// add job
	select {
	case d.jobs <- progress:
		jsend.Status = Success
		jsend.Data = progress
	default:
		jsend.Message = "Que is full, please try again later"
		jsend.ErrorCode = ErrCodeFullQue
	}
	jsend.write(w)
}

func (d *Delegator) getProgress(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	jsend := &JSend{}
	const keyID = "service_progress_token"
	token := r.Header.Get(keyID)
	if token == "" {
		jsend.Message = "you must include the token given to you when calling POST /service. The key=" + keyID
		jsend.write(w)
		return
	}

	var progress *serviceCreationStatus
	d.RLock()
	for i := range d.records {
		if token == d.records[i].Token {
			progress = d.records[i]
			break
		}
	}
	d.RUnlock()

	if progress == nil {
		jsend.Status = Fail
		jsend.Message = "service creation job is still in que or the token is unknown. Try again later"
	} else {
		jsend.Status = Success
		jsend.Data = progress

		progress.RLock()
		defer progress.RUnlock()
	}
	jsend.write(w)
}

func runServer(d *Delegator) {

	// web server
	port := os.Getenv("WEB_SERVER_PORT")
	if port == "" {
		panic("missing environment variable WEB_SERVER_PORT")
	}

	router := httprouter.New()
	router.GET("/health", func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		fmt.Fprintf(w, `{"status":"ok"}`)
	})

	router.POST("/service", d.createService)
	router.GET("/progress", d.getProgress)

	log.Fatal(http.ListenAndServe(":"+port, router))
	d.shutdown()
}
