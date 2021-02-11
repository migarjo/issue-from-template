package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
	"time"

	yaml "gopkg.in/yaml.v2"
)

type issue struct {
	*request
	*date
	endpoint string
	template string
}

func NewIssue() *issue {
	i := &issue{}
	i.request = NewRequest(201)
	if os.Getenv("ADD_DATES") == "" {
		i.date = NewDate(time.Now())
	} else {
		dates, err := strconv.Atoi(os.Getenv("ADD_DATES"))
		if err != nil {
			return nil
		}
		i.date = NewDate(time.Now().AddDate(0, 0, dates))
	}
	if os.Getenv("GITHUB_REPOSITORY_OVERRIDE") != "" {
		i.endpoint = "https://api.github.com/repos/" + os.Getenv("GITHUB_REPOSITORY_OVERRIDE") + "/issues"
	} else {
		i.endpoint = "https://api.github.com/repos/" + os.Getenv("GITHUB_REPOSITORY") + "/issues"
	}
	i.template = filepath.Join(os.Getenv("GITHUB_WORKSPACE"), ".github", "ISSUE_TEMPLATE", os.Getenv("IFT_TEMPLATE_NAME"))
	return i
}

func (i *issue) parseTemplate() (string, error) {
	file, err := ioutil.ReadFile(i.template)
	if err != nil {
		return "", err
	}

	t, err := template.New("body").Parse(string(file))
	if err != nil {
		return "", err
	}

	b := new(bytes.Buffer)
	err = t.Execute(b, i.date)
	if err != nil {
		return "", err
	}

	return b.String(), nil
}

func (i *issue) generatePayload() ([]byte, error) {
	templateBody, err := i.parseTemplate()
	if err != nil {
		return nil, err
	}

	s := strings.Split(templateBody, "---\n")

	t := &struct {
		Name      string      `yaml:"name"`
		About     string      `yaml:"about"`
		Title     string      `yaml:"title"`
		Labels    StringSlice `yaml:"labels"`
		Assignees StringSlice `yaml:"assignees"`
	}{}

	err = yaml.UnmarshalStrict([]byte(s[1]), t)
	if err != nil {
		return nil, err
	}

	payload := &struct {
		Title     string   `json:"title"`
		Body      string   `json:"body"`
		Assignees []string `json:"assignees"`
		Labels    []string `json:"labels"`
	}{}

	payload.Title = strings.Replace(t.Title, "[GITHUB_HANDLE]", os.Getenv("NEW_ASSIGNEE"), 1)
	payload.Body = strings.Replace(s[2], "\n", "", 1)
	payload.Assignees = t.Assignees.splitAndTrimSpace()
	payload.Labels = t.Labels.splitAndTrimSpace()
	
	payload.Assignees = append(payload.Assignees, os.Getenv("NEW_ASSIGNEE"))

	d, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	return d, nil
}

func (i *issue) post() (string, error) {
	data, err := i.generatePayload()
	if err != nil {
		return "", err
	}

	response, err := i.request.post(data, i.endpoint)
	if err != nil {
		return "", err
	}

	r := &struct {
		CommentsURL string `json:"comments_url"`
	}{}

	err = json.Unmarshal(response, r)
	if err != nil {
		return "", err
	}

	if r.CommentsURL == "" {
		return "", errors.New("comments_url is missing in the response")
	}

	return r.CommentsURL, nil
}
