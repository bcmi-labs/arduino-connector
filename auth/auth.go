//
//  This file is part of arduino-connector
//
//  Copyright (C) 2017-2018  Arduino AG (http://www.arduino.cc/)
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

// Package auth uses the `oauth2 authorization_code` flow to authenticate with Arduino
//
// If you have the username and password of a user, you can just instantiate a client with sane defaults:
//
//   client := auth.New()
//
// and then call the Token method to obtain a Token object with an AccessToken and a RefreshToken
//
//   token, err := client.Token(username, password)
//
// If instead you already have a token but want to refresh it, just call
//
//   token, err := client.refresh(refreshToken)
package auth

import (
	"encoding/json"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/pkg/errors"
)

type DeviceCode struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
	VerificationURIComplete string `json:"verification_uri_complete"`
}

// HTTPClient interface
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
	Get(url string) (resp *http.Response, err error)
}

var (
	client HTTPClient
)

// Init initialize correctly HTTP client
func Init() {
	client = &http.Client{}
}

// StartDeviceAuth request with POST auth using clientID
func StartDeviceAuth(authURL, clientID string) (data DeviceCode, err error) {
	url := authURL + "/oauth/device/code"

	payload := strings.NewReader("client_id=" + clientID + "&audience=https://api.arduino.cc")

	req, err := http.NewRequest("POST", url, payload)
	if err != nil {
		return data, err
	}

	req.Header.Add("content-type", "application/x-www-form-urlencoded")

	res, err := client.Do(req)
	if err != nil {
		return data, err
	}

	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return data, err
	}

	err = json.Unmarshal(body, &data)
	if err != nil {
		return data, err
	}

	return data, nil
}

func CheckDeviceAuth(authURL, clientID, deviceCode string) (token string, err error) {
	url := authURL + "/oauth/token"

	payload := strings.NewReader("grant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Adevice_code&device_code=" + deviceCode + "&client_id=" + clientID)

	req, err := http.NewRequest("POST", url, payload)
	if err != nil {
		return token, err
	}

	req.Header.Add("content-type", "application/x-www-form-urlencoded")

	res, err := client.Do(req)
	if err != nil {
		return token, err
	}

	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return token, err
	}

	if res.StatusCode != 200 {
		return token, errors.New(string(body))
	}

	data := struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		TokenType   string `json:"token_type"`
	}{}

	err = json.Unmarshal(body, &data)
	if err != nil {
		return token, err
	}

	return data.AccessToken, nil
}

// Config contains the variables you may want to change
type Config struct {
	// CodeURL is the endpoint to redirect to obtain a code
	CodeURL string

	// TokenURL is the endpoint where you can request an access code
	TokenURL string

	// ClientID is the client id you are using
	ClientID string

	// RedirectURI is the redirectURI where the oauth process will redirect. It's only required since the oauth system checks for it, but we intercept the redirect before hitting it
	RedirectURI string

	// Scopes is a space-separated list of scopes to require
	Scopes string
}

// New returns an auth configuration with sane defaults
func New() *Config {
	return &Config{
		CodeURL:     "https://hydra.arduino.cc/oauth2/auth",
		TokenURL:    "https://hydra.arduino.cc/oauth2/token",
		ClientID:    "cli",
		RedirectURI: "http://localhost:5000",
		Scopes:      "profile:core offline",
	}
}

// Token is the response of the two authentication functions
type Token struct {
	// Access is the token to use to authenticate requests
	Access string `json:"access_token"`

	// Refresh is the token to use to request another access token. It's only returned if one of the scopes is "offline"
	Refresh string `json:"refresh_token"`

	// TTL is the number of seconds that the tokens will last
	TTL int `json:"expires_in"`

	// Scopes is a space-separated list of scopes associated to the access token
	Scopes string `json:"scope"`

	// Type is the type of token
	Type string `json:"token_type"`
}

// Token authenticates with the given username and password and returns a Token object
func (c *Config) Token(user, pass string) (*Token, error) {
	// We want to make sure we send the proper cookies each step, so we don't follow redirects
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	// Request authentication page
	url, cookies, err := c.requestAuth(client)
	if err != nil {
		return nil, errors.Wrap(err, "get the auth page")
	}

	// Authenticate
	code, err := c.authenticate(client, cookies, url, user, pass)
	if err != nil {
		return nil, errors.Wrap(err, "authenticate")
	}

	// Request token
	token, err := c.requestToken(client, code)
	if err != nil {
		return nil, errors.Wrap(err, "request token")
	}
	return token, nil
}

// Refresh exchanges a token for a new one
func (c *Config) Refresh(token string) (*Token, error) {
	client := http.Client{}
	query := url.Values{}
	query.Add("refresh_token", token)
	query.Add("client_id", c.ClientID)
	query.Add("redirect_uri", c.RedirectURI)
	query.Add("grant_type", "refresh_token")

	req, err := http.NewRequest("POST", c.TokenURL, strings.NewReader(query.Encode()))
	if err != nil {
		return nil, err
	}

	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth("cli", "")
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	data := Token{}

	err = json.Unmarshal(body, &data)
	if err != nil {
		return nil, err
	}
	return &data, nil
}

// cookies keeps track of the cookies for each request
type cookies map[string][]*http.Cookie

// requestAuth calls hydra and follows the redirects until it reaches the authentication page. It saves the cookie it finds so it can apply them to subsequent requests
func (c *Config) requestAuth(client HTTPClient) (string, cookies, error) {
	uri, err := url.Parse(c.CodeURL)
	if err != nil {
		return "", nil, err
	}

	query := uri.Query()
	query.Add("client_id", c.ClientID)
	query.Add("state", randomString(8))
	query.Add("scope", c.Scopes)
	query.Add("response_type", "code")
	query.Add("redirect_uri", c.RedirectURI)
	uri.RawQuery = query.Encode()

	// Navigate to hydra request page
	res, err := client.Get(uri.String())
	if err != nil {
		return "", nil, err
	}

	cs := cookies{}
	cs["hydra"] = res.Cookies()

	// Navigate to auth request page
	res, err = client.Get(res.Header.Get("Location"))
	if err != nil {
		return "", nil, err
	}

	cs["auth"] = res.Cookies()
	return res.Request.URL.String(), cs, err
}

var errorRE = regexp.MustCompile(`<div class="error">(?P<error>.*)</div>`)

// authenticate uses the user and pass to pass the authentication challenge and returns the authorization_code
func (c *Config) authenticate(client HTTPClient, cookies cookies, uri, user, pass string) (string, error) {
	// Find csrf
	csrf := ""
	for _, cookie := range cookies["auth"] {
		if cookie.Name == "_csrf" && cookie.Value != "" {
			csrf = cookie.Value
			break
		}
	}
	query := url.Values{}
	query.Add("username", user)
	query.Add("password", pass)
	query.Add("csrf", csrf)
	query.Add("g-recaptcha-response", "")

	req, err := http.NewRequest("POST", uri, strings.NewReader(query.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	// Apply cookies
	for _, cookie := range cookies["auth"] {
		req.AddCookie(cookie)
	}

	res, err := client.Do(req)
	if err != nil {
		return "", err
	}

	if res.StatusCode != 302 {
		body, errRead := ioutil.ReadAll(res.Body)
		if errRead != nil {
			return "", errRead
		}

		errs := errorRE.FindStringSubmatch(string(body))
		if len(errs) < 2 {
			return "", errors.New("status = " + res.Status + ", response = " + string(body))
		}
		return "", errors.New(errs[1])
	}

	// Follow redirect to hydra
	req, err = http.NewRequest("GET", res.Header.Get("Location"), nil)
	if err != nil {
		return "", err
	}

	for _, cookie := range cookies["hydra"] {
		req.AddCookie(cookie)
	}

	res, err = client.Do(req)
	if err != nil {
		return "", err
	}

	redir, err := url.Parse(res.Header.Get("Location"))
	if err != nil {
		return "", err
	}

	return redir.Query().Get("code"), nil
}

func (c *Config) requestToken(client HTTPClient, code string) (*Token, error) {
	query := url.Values{}
	query.Add("code", code)
	query.Add("client_id", c.ClientID)
	query.Add("redirect_uri", c.RedirectURI)
	query.Add("grant_type", "authorization_code")

	req, err := http.NewRequest("POST", c.TokenURL, strings.NewReader(query.Encode()))
	if err != nil {
		return nil, err
	}

	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(c.ClientID, "")
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	if res.StatusCode != 200 {
		data := struct {
			Error string `json:"error_description"`
		}{}
		err = json.Unmarshal(body, &data)
		if err != nil {
			return nil, err
		}
		return nil, errors.New(data.Error)
	}

	data := Token{}

	err = json.Unmarshal(body, &data)
	if err != nil {
		return nil, err
	}
	return &data, nil
}

// randomString generates a string of random characters of fixed length.
// stolen shamelessly from https://stackoverflow.com/questions/22892120/how-to-generate-a-random-string-of-a-fixed-length-in-golang
const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
const (
	letterIdxBits = 6                    // 6 bits to represent a letter index
	letterIdxMask = 1<<letterIdxBits - 1 // All 1-bits, as many as letterIdxBits
	letterIdxMax  = 63 / letterIdxBits   // # of letter indices fitting in 63 bits
)

var src = rand.NewSource(time.Now().UnixNano())

func randomString(n int) string {
	b := make([]byte, n)
	// A src.Int63() generates 63 random bits, enough for letterIdxMax characters!
	for i, cache, remain := n-1, src.Int63(), letterIdxMax; i >= 0; {
		if remain == 0 {
			cache, remain = src.Int63(), letterIdxMax
		}
		if idx := int(cache & letterIdxMask); idx < len(letterBytes) {
			b[i] = letterBytes[idx]
			i--
		}
		cache >>= letterIdxBits
		remain--
	}

	return string(b)
}
