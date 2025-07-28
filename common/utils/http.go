package utils

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"moul.io/http2curl"
)

var client = &http.Client{}

const (
	HttpPostMethod = "POST"
	HttpGetMethod  = "GET"
)

func HttpPost(ctx context.Context, uri string, bodyStr string, protocol string, domain string) (string, error) {
	return HttpDo(ctx, uri, HttpPostMethod, bodyStr, protocol, domain)
}

func HttpGet(ctx context.Context, uri string, query string, protocol string, domain string) (string, error) {
	return HttpDo(ctx, uri, HttpGetMethod, query, protocol, domain)
}

func HttpDo(ctx context.Context, uri string, method string, bodyStr string, protocol string, domain string) (string, error) {
	req, err := http.NewRequest(method, fmt.Sprintf("%s://%s%s", protocol, domain, uri), strings.NewReader(bodyStr))
	if err != nil {
		println(err.Error())
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	command, _ := http2curl.GetCurlCommand(req)
	println("curl: ", command.String())

	resp, err := client.Do(req)
	if err != nil {
		println(err.Error())
		return "", err
	}

	if resp == nil {
		return "", fmt.Errorf("resp nil")
	}

	if resp.StatusCode != 200 {
		println(resp.StatusCode)
		return "", fmt.Errorf("http code not success")
	}

	body, err := ioutil.ReadAll(resp.Body)

	defer func() {
		err = resp.Body.Close()
		if err != nil {
			println("[HttpDo] response close error: %v, req: %v", err, req)
		}
	}()

	if err != nil {
		// handle error
		return "", err
	}

	println("http utils get ", string(body))
	println("", resp.Header.Get("X-Tt-Logid"))
	return string(body), nil
}
