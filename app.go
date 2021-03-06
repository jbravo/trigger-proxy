package main

import (
	"crypto/tls"
	"encoding/csv"
	"errors"
	"flag"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	exitFail = 1
)

var (
	mapping    = make(map[string][]string)
	timeKeeper = make(map[string]*time.Timer)

	JenkinsURL   string
	JenkinsUser  string
	JenkinsToken string
	JenkinsMulti string
	MappingFile  string
	QuietPeriod  int
	FileMatching bool
)

type triggerMapping struct {
	mapping map[string][]string
}

func triggerJob(job string) bool {
	url := createJobURL(JenkinsURL, job)

	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return false
	}

	// if user and token is defined, use it for basic auth
	if JenkinsUser != "" {
		req.SetBasicAuth(JenkinsUser, JenkinsToken)
	} else {
		// otherwise use the token for the direct build trigger
		url = string(url + "?token=" + JenkinsToken)
	}

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	timeout := time.Duration(5 * time.Second)
	client := &http.Client{Transport: tr, Timeout: timeout}
	resp, err := client.Do(req)

	if err != nil {
		log.Print("Error:", err)

		return false
	}

	if !(200 <= resp.StatusCode && resp.StatusCode <= 299) {
		log.Printf("... %v failed with status code %v\n", job, resp.StatusCode)
	} else {
		log.Printf("... %v triggered\n", job)
	}

	return true
}

func createJobURL(jenkinsURL, job string) string {
	return string(jenkinsURL + "/job/" + job + "/build")
}

func createTimer(job string) {
	if _, ok := timeKeeper[job]; ok {
		log.Print("Reseting timer for job ", job)
		timeKeeper[job].Stop()
		delete(timeKeeper, job)
	}

	log.Printf("Creating timer for job '%s' with quiet period of %d seconds", job, QuietPeriod)

	timer := time.AfterFunc(time.Second*time.Duration(QuietPeriod), func() {
		log.Print("Quiet period exceeded for job ", job)
		triggerJob(job)
		if _, ok := timeKeeper[job]; ok {
			log.Print("Deleting timer for job ", job)
			delete(timeKeeper, job)
		}
	})

	timeKeeper[job] = timer
	if _, ok := timeKeeper[job]; ok {
		log.Print("Timer saved in time keeper")
	}

	return
}

func ParseGetRequest(r *http.Request) (string, string, []string, error) {
	repo := ""
	branch := ""
	files := []string{}

	log.Print("parsing get request")
	repos, ok := r.URL.Query()["repo"]

	if !ok || len(repos) < 1 {
		log.Print("Repo is missing")
		log.Print("Aborting request handling")

		return repo, branch, files, errors.New("repo is missing")
	}

	repo = repos[0]

	log.Print("Parsed repo:", repo)

	branchs, ok := r.URL.Query()["branch"]

	if !ok || len(branchs) < 1 {
		log.Print("Branch is missing. Assuming master")
		branch = "master"
	} else {
		branch = branchs[0]
	}

	log.Print("Parsed branch: ", branch)

	return repo, branch, files, nil
}

func handler(w http.ResponseWriter, r *http.Request) {
	log.Print("Handling new request")

	repo, branch, files, err := ParseGetRequest(r)

	if err != nil {
		log.Print("Aborting request handling")

		return
	}

	log.Print("Files: ", files)

	key := BuildMappingKey([]string{repo, branch})

	log.Print("Searching mappings for key: ", key)

	if len(mapping[key]) == 0 {
		log.Print("No mappings found")
		log.Print("Aborting request handling")
		return
	}

	log.Print("Number of mappings found: ", len(mapping[key]))

	log.Print("Start processing mappings")
	for _, job := range mapping[key] {
		createTimer(job)
	}
	log.Print("End processing mappings")

	log.Print("Handling request finished")
}

func main() {
	if err := run(os.Args, os.Stdout); err != nil {
		log.Fatalf("%s\n", err)
		os.Exit(exitFail)
	}
}

func parseFlags(args []string) {
	flag.StringVar(&JenkinsURL, "jenkins-url", "", "sets the jenkins url")
	flag.StringVar(&JenkinsUser, "jenkins-user", "", "jenkins username")
	flag.StringVar(&JenkinsToken, "jenkins-token", "", "token for user or root token to trigger anonymously")
	flag.StringVar(&JenkinsMulti, "jenkins-multi", "", "root folder or job name")
	flag.StringVar(&MappingFile, "mappingfile", "mapping.csv", "path to the mapping file")
	flag.IntVar(&QuietPeriod, "quietperiod", 10, "defines the time trigger-proxy will wait until the job is triggered")
	flag.BoolVar(&FileMatching, "filematch", false, "try to match for file names")

	flag.Parse()
}

func run(args []string, stdout io.Writer) error {
	log.Println("Starting trigger-proxy ...")

	log.Println("Checking environment variables")

	if JenkinsURL == "" {
		return errors.New("No JENKINS_URL defined")
	}

	if JenkinsUser == "" {
		log.Println("No JENKINS_USER defined")
	}

	if JenkinsToken == "" {
		return errors.New("No JENKINS_TOKEN defined")
	}

	if JenkinsMulti != "" {
		log.Printf("Found multibranch project: %s\n", JenkinsMulti)

		JenkinsURL = JenkinsURL + "/job/" + JenkinsMulti
	}

	log.Printf("Found configured quiet period: %d\n", QuietPeriod)
	log.Printf("Project URL: %s\n", JenkinsURL)

	log.Printf("Found configured mapping file: %s\n", MappingFile)

	if err := ProcessMappingFile(MappingFile); err != nil {
		return err
	}

	http.HandleFunc("/", handler)

	log.Println("Serving on port 8080")
	http.ListenAndServe(":8080", nil)

	return nil
}

// ProcessMappingFile processes the file at given path
func ProcessMappingFile(mappingfile string) error {
	log.Printf("Reading mapping from file: %s\n", mappingfile)

	file, err := os.Open(mappingfile)
	if err != nil {
		return err
	}
	defer file.Close()

	tm, perr := ParseMappingFile(file, FileMatching)

	if perr != nil {
		return err
	}

	mapping = tm.mapping

	return nil
}

// ParseMappingFile parses the given file and returns the mapping
func ParseMappingFile(file io.Reader, filematch bool) (triggerMapping, error) {
	var m = make(map[string][]string)

	reader := csv.NewReader(file)
	reader.Comma = ';'
	lineCount := 0
	for {
		record, err := reader.Read()

		if err == io.EOF {
			break
		} else if err != nil {
			return triggerMapping{mapping: nil}, err
		}

		var key string
		if filematch {
			if len(record) != 4 {
				return triggerMapping{mapping: nil}, errors.New("no file matching information provided in mapping file")
			}
			key = BuildMappingKey([]string{record[0], record[1], record[3]})
		} else {
			key = BuildMappingKey([]string{record[0], record[1]})
		}
		m[key] = append(m[key], record[2])
		lineCount++
	}

	log.Printf("Successfully read mappings: %d\n", lineCount)

	return triggerMapping{mapping: m}, nil
}

// BuildMappingKey returns the mapping for a given set of strings
func BuildMappingKey(keys []string) string {
	return strings.Join(keys, "|")
}
