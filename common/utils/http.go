package utils

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
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
	req, err := http.NewRequestWithContext(ctx, method, fmt.Sprintf("%s://%s%s", protocol, domain, uri), strings.NewReader(bodyStr))
	if err != nil {
		println(err.Error())
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

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
		// HTTP 请求失败时向调用方返回错误。
		return "", err
	}
	return string(body), nil
}
