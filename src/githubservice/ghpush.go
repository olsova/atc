package githubservice

import (
	"context"
	"encoding/xml"
	"github.com/google/go-github/github"
	"log"
	"net/http"
	"time"
)

type PoxXml struct {
	Version string `xml:"version"`
}

func getVersionFromPomXml(content string) (string, error) {
	pom := &PoxXml{}
	if err := xml.Unmarshal([]byte(content), pom); err != nil { return "", err }
	return pom.Version, nil
}

func PushAction(push *github.WebHookPayload, id int64) {
	const versionSource = "pom.xml"

	token, err := getAccessToken(id)
	if err != nil {
		log.Printf("getAccessToken Error: %v", err)
	}
	ctx := context.Background()
	client := getGithubClient( token, ctx)
	owner := push.GetRepo().GetOwner().GetName()
	repo := push.GetRepo().GetName()

	old, _, _, err := client.Repositories.GetContents(ctx, owner, repo, versionSource, &github.RepositoryContentGetOptions{Ref: push.GetBefore()})
	if err != nil {
		log.Printf("get old content error: %v", err)
		return
	}
	oldContent, _ := old.GetContent()
	oldVersion,_ := getVersionFromPomXml(oldContent)

	f, _, resp, err := client.Repositories.GetContents( ctx, owner, repo, versionSource, nil)
	if err != nil {
		log.Printf("get contents error: %v", err)
		return
	}

	if resp.StatusCode !=  http.StatusOK {
		log.Printf("Wrong access status during getContent for installation %d: %s", id, resp.Status)
		return
	}
	newContent, _ := f.GetContent()
	newVersion,_ := getVersionFromPomXml(newContent)

	if newVersion != oldVersion {
		log.Printf("There is a new version! Old version: %q, new version: %q", oldVersion, newVersion)

		caption := "v"+newVersion
		sha := push.GetAfter()
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
			log.Printf("addTagToCommit Error: %v", err)
			return
		}

		cmnt := "Comment for " + newVersion
		_, _, err = client.Repositories.CreateComment(context.Background(), owner, repo, sha, &github.RepositoryComment{
			Body:      &cmnt,
		})
		if err != nil {
			log.Printf("add comment error :%v", err)
		}
	}
}