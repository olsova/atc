package githubservice

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/google/go-github/v39/github"
)

type TagVersion struct {
	Version string
}

const (
	behaviorBefore = "before"
)

var autoFetchers = map[string]VersionFetcher{
	"pom.xml":           &pomXmlFetcher{},
	"gradle.properties": &gradlePropertiesFetcher{},
	".npmrc":            &npmrcFetcher{},
}

func detectFetchType(path string) string {
	if path == "" {
		return ""
	}
	return filepath.Base(path)
}

func madeСaptionToTemplate(templateString string, tagVersion TagVersion) (string, error) {
	buf := new(bytes.Buffer)
	tmpl, err := template.New("template tagVersion").Parse(templateString)
	if err != nil {
		return "", err
	}
	err = tmpl.Execute(buf, tagVersion)
	if err != nil {
		return "", err
	}
	if buf.String() == "" {
		return "v" + tagVersion.Version, nil
	}
	return buf.String(), nil
}

func madeShaToBehavior(push *github.WebHookPayload, behavior string) *string {
	if strings.ToLower(behavior) == behaviorBefore {
		return push.Before
	}
	return push.After
}

func PushAction(push *github.WebHookPayload, clientProvider ClientProvider) {
	id := *push.Installation.ID

	token, err := getAccessToken(id, clientProvider)
	if err != nil {
		log.Printf("getAccessToken Error: %v", err)
		return
	}
	owner := push.GetRepo().GetOwner().GetName()
	repo := push.GetRepo().GetName()
	fullname := push.GetRepo().GetFullName()
	ctx := context.Background()
	client := clientProvider.Get(token, ctx)

	ghOldContentProviderPtr := &ghContentProvider{
		owner:    owner,
		repo:     repo,
		sha1:     push.GetBefore(),
		ctx:      ctx,
		ghClient: client,
	}
	ghNewContentProviderPtr := &ghContentProvider{
		owner:    owner,
		repo:     repo,
		ctx:      ctx,
		ghClient: client,
	}

	settings, err := getAtcSetting(ghNewContentProviderPtr)
	commitComment := ""
	if err != nil {
		commitComment := fmt.Sprint(err)
		addComment(client, owner, repo, push.GetAfter(), commitComment)
	} else {
		newVersion := TagVersion{}
		oldVersion := TagVersion{}
		fetchType := detectFetchType(settings.Path)

		if fetchType != "" {
			var err error
			var reqError *RequestError
			fetcher := autoFetchers[fetchType]
			err = fetcher.GetVersion(ghOldContentProviderPtr, settings.Path, &oldVersion)
			if err != nil && err != errHttpStatusCode { //ignore http api error
				log.Printf("get prev version error for %q: %v", fullname, err)
				addComment(client, owner, repo, push.GetAfter(), fmt.Sprintf("file %s with old version not found", fetchType))
				return
			}
			err = fetcher.GetVersion(ghNewContentProviderPtr, settings.Path, &newVersion)
			if err != nil {
				if err == errHttpStatusCode {
					log.Printf("Wrong access status during getContent for installation %d for %q: %d", id, fullname, reqError.StatusCode)
				} else {
					log.Printf("get version error for %q: %v", fullname, err)
					addComment(client, owner, repo, push.GetAfter(), fmt.Sprintf("file %s with new version not found", fetchType))
				}
				return
			}
		} else {
			commitComment = "File .atc.yaml not found. "
			fetched := false
			for defaultPath, fetcher := range autoFetchers {
				var err error
				err = fetcher.GetVersionDefaultPath(ghOldContentProviderPtr, &oldVersion)
				if err != nil && err != errHttpStatusCode { //ignore http api error
					log.Printf("get prev version error for %q: %v", fullname, err)
					continue
				}

				err = fetcher.GetVersionDefaultPath(ghNewContentProviderPtr, &newVersion)

				if err == nil {
					fetched = true
					commitComment += "Used default settings. "
					break
				} else {
					log.Printf("autofetcher error for %q: %v", defaultPath, err)
				}
			}
			if !fetched {
				commitComment += "Not found supported package manager."
				addComment(client, owner, repo, push.GetAfter(), commitComment)
				log.Printf("Unable to fetch version using known methods!") //probably should be comment
				return
			}
		}

		if newVersion != oldVersion {
			log.Printf("There is a new version for %q! Old version: %q, new version: %q", fullname, oldVersion, newVersion)
			caption, err := madeСaptionToTemplate(settings.Template, newVersion)
			if err != nil {
				log.Printf("error in go templates: %v", err)
				return
			}
			sha := *madeShaToBehavior(push, settings.Behavior)
			objType := "commit"
			timestamp := time.Now()

			tag := &github.Tag{
				Tag:     &caption,
				Message: &caption,
				Tagger: &github.CommitAuthor{
					Date:  &timestamp,
					Name:  push.GetPusher().Name,
					Email: push.GetPusher().Email,
					Login: push.GetPusher().Login,
				},
				Object: &github.GitObject{
					Type: &objType,
					SHA:  &sha,
				},
			}

			if err := addTagToCommit(client, owner, repo, tag); err != nil {
				log.Printf("addTagToCommit Error for %q: %v", fullname, err)
				addComment(client, owner, repo, sha, fmt.Sprintf("can't add tag to commit, error : %v", err))
				return
			}

			commitComment += fmt.Sprintf("Added a new version for %q: %q", fullname, caption)
			addComment(client, owner, repo, sha, commitComment)
		}
	}
}
