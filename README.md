# trigger-proxy

trigger-proxy is used to map git repository / branch tuples to job names on Jenkins. Especially, but not exclusive for multibranch pipeline projects.
This is useful to have commit hook triggered builds on pipelines without direct git association in Jenkins.

## Badges

[![Go Report Card](https://goreportcard.com/badge/github.com/vebis/trigger-proxy)](https://goreportcard.com/report/github.com/vebis/trigger-proxy)

## Getting started

Set the following environment variables

* JENKINS_URL - your jenkins installation
* JENKINS_MULTI - name of multibranch pipeline project
* JENKINS_USER - user who can trigger builds
* JENKINS_TOKEN - the api token of the user
* JENKINS_QUIET - quiet period for jobs, defaults to 30 (seconds)
* MAPPING_FILE - path to mapping file, defaults to mapping.csv

## Usage

```bash
sudo docker run -e JENKINS_URL="https://jenkins:8443" -e JENKINS_MULTI="builds" -e JENKINS_USER="triggeruser" -e JENKINS_TOKEN="token" vebis/trigger-proxy
```

Send an http request with GET parameter "repo" to port 8080. If you defined GET parameter branch it will be considered, otherwise "master" ist assumed.
The app will lookup any job names for your input and will trigger them.

## Authors

* **Stephan Kirsten**

## License

BSD 2-Clause "Simplified" License
