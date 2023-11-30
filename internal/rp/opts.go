package rp

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/tidwall/gjson"
)

type RequestMatcher func(*http.Request) bool
type ResponseTransformer func(*http.Response) *http.Response

func RespondWithError(statusCode int, body string) ResponseTransformer {
	return nil
}

func WithUserInfo(t *testing.T, hsURL, userID, deviceID string) RequestMatcher {
	c := &http.Client{
		Timeout: 10 * time.Second,
	}
	accessToken := ""
	return func(req *http.Request) bool {
		if accessToken == "" {
			// figure out who this is
			whoami, err := http.NewRequest("GET", fmt.Sprintf("%s/_matrix/client/v3/account/whoami", hsURL), nil)
			if err != nil {
				t.Errorf("WithUserInfo: failed to create /whoami request: %s", err) // should be unreachable
			}
			// discard all errors and just don't set the access token. We expect to see some errors here
			// as we will be hitting hsURL for users not on that HS.
			res, _ := c.Do(whoami)
			if res.StatusCode == 200 {
				body, err := io.ReadAll(res.Body)
				if err != nil {
					t.Errorf("WithUserInfo: failed to read /whoami response: %s", err)
				}
				res.Body.Close()
				if !gjson.ValidBytes(body) {
					t.Errorf("WithUserInfo: /whoami response is not JSON: %s", string(body))
				}
				bodyJSON := gjson.ParseBytes(body)
				if userID == bodyJSON.Get("user_id").Str && deviceID == bodyJSON.Get("device_id").Str {
					accessToken = strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer ")
					t.Logf("WithUserInfo: identified user with token %s", accessToken)
				}
			}
		}
		if strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer ") == accessToken {
			return true
		}
		return false // some other user
	}
}

func WithPathSuffix(path string) RequestMatcher {
	return func(req *http.Request) bool {
		return strings.HasSuffix(req.URL.Path, path)
	}
}

func WithRepititions(num int) RequestMatcher {
	seen := 0
	return func(req *http.Request) bool {
		allowed := seen < num
		seen++
		return allowed
	}
}
