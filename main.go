package main

import (
	"flag"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/evermax/stargraph/github"
	"github.com/evermax/stargraph/lib"
	"github.com/evermax/stargraph/service"
	"github.com/evermax/stargraph/service/newrepo"
)

const (
	accessTokenURL = "https://github.com/login/oauth/access_token"
)

var conf string

func init() {
	flag.StringVar(&conf, "conf", "conf", "The toml conf file to read from. Default is \"conf\"")
}

type config struct {
	ClientID     string
	ClientSecret string
	Host         string
	Port         string
	WorkerNum    int
	dispatcher   *service.Dispatcher
}

func main() {
	flag.Parse()
	var config = readConfig(conf)
	config.dispatcher = service.NewDispatcher(config.WorkerNum, config.WorkerNum)

	config.dispatcher.Run()
	defer config.dispatcher.Stop()

	http.HandleFunc("/api", config.apiHandler)
	http.HandleFunc("/callback", config.callbackHander)
	log.Println(http.ListenAndServe(config.Host+":"+config.Port, nil))
}

func readConfig(configfile string) config {
	_, err := os.Stat(configfile)
	if err != nil {
		log.Fatal("Config file is missing: ", configfile)
	}

	var conf config
	if _, err := toml.DecodeFile(configfile, &conf); err != nil {
		log.Fatal(err)
	}
	return conf
}

func (c config) apiHandler(w http.ResponseWriter, r *http.Request) {
	var repo = r.FormValue("repo")
	var token = r.FormValue("token")

	info, err := github.GetRepoInfo(token, repo)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	var timestamps []int64
	timestamps, err = newrepo.GetAllTimestamps(c.dispatcher.JobQueue, 100, token, info)

	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	err = lib.WriteCanvasJS(timestamps, info, w)
	if err != nil {
		log.Println(err)
	}
}

func (c config) callbackHander(w http.ResponseWriter, r *http.Request) {
	code := r.FormValue("code")
	if code == "" {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	log.Print(code)
	resp, err := http.PostForm(accessTokenURL, url.Values{
		"client_id":     []string{c.ClientID},
		"client_secret": []string{c.ClientSecret},
		"code":          []string{code},
	})
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if resp != nil {
		defer resp.Body.Close()
	}

	var bodyBytes []byte
	bodyBytes, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	var token string
	var params = strings.Split(string(bodyBytes), "&")
	for _, param := range params {
		if strings.Contains(param, "access_token") {
			tokenParam := strings.Split(param, "=")
			token = tokenParam[1]
			break
		}
	}
	w.Header().Set("Set-Cookie", "token="+token)
	http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
}
