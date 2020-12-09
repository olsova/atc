package githubservice

import (
	"context"
	"envvars"
	"errors"
	"io/ioutil"
	"log"
	"net/http"
	"os"
)

var (
	errWrongCreateAccessTokenStatus = errors.New("wrong access status during create access token for installation (not 201)")
)

func getAccessToken(id int64) (string, error) {
	var pemData []byte
	var err error
	pemEnv := os.Getenv(envvars.PemData)
	if pemEnv == "" {
		pemPath := os.Getenv(envvars.PemPathVariable)
		if pemPath == "" { return "", errNoPemEnv}
		pemData, err = ioutil.ReadFile(pemPath)
		if err != nil { return "", err }
		log.Printf("ATC uses pem from file: %q", pemPath)
	} else {
		pemData = []byte(pemEnv)
		log.Print("ATC uses pem data from environment variable")
	}

	jwt, err := getJwt(pemData)
	if err != nil {
		return "", err
	}

	ctx := context.Background()
	client := getGithubClient(jwt, ctx)
	inst, resp, err := client.Apps.CreateInstallationToken(ctx, id, nil)
	if err != nil { return "", err }
	if resp.StatusCode !=  http.StatusCreated {
		return "", errWrongCreateAccessTokenStatus
	}

	return inst.GetToken(), nil
}