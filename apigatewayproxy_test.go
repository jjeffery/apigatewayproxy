package apigatewayproxy

import (
	"io/ioutil"
	"net/http"
	"os"
	"reflect"
	"testing"
	"unicode/utf8"

	"github.com/aws/aws-lambda-go/events"
	"github.com/davecgh/go-spew/spew"
)

func TestSpike(t *testing.T) {
	var b []byte
	if want, got := "", string(b); want != got {
		t.Errorf("want=%q, got=%q", want, got)
	}

	if want, got := true, utf8.Valid(b); want != got {
		t.Errorf("want=%v, got=%v", want, got)
	}
}

func TestIsLambda(t *testing.T) {
	os.Setenv("_LAMBDA_SERVER_PORT", "3000")
	if got, want := IsLambda(), true; got != want {
		t.Errorf("got=%v, want=%v", got, want)
	}
	os.Unsetenv("_LAMBDA_SERVER_PORT")
	if got, want := IsLambda(), false; got != want {
		t.Errorf("got=%v, want=%v", got, want)
	}
}

func TestHandler(t *testing.T) {
	tests := []struct {
		handler  http.Handler
		request  events.APIGatewayProxyRequest
		response events.APIGatewayProxyResponse
		err      error
	}{
		{
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte("hello"))
			}),
			request: events.APIGatewayProxyRequest{
				Path:       "/test",
				HTTPMethod: "GET",
			},
			response: events.APIGatewayProxyResponse{
				StatusCode: 200,
				Headers:    map[string]string{},
				Body:       "hello",
			},
		},
		{
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("content-type", "text/plain")
				w.Write([]byte("hello"))
			}),
			request: events.APIGatewayProxyRequest{},
			response: events.APIGatewayProxyResponse{
				StatusCode: 200,
				Headers: map[string]string{
					"Content-Type": "text/plain",
				},
				Body: "hello",
			},
		},
		{
			// URL set correctly from path and query
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("content-type", "text/plain")
				w.Write([]byte(r.URL.String()))
				w.Write([]byte("\n"))
				w.Write([]byte(r.Method))
			}),
			request: events.APIGatewayProxyRequest{
				Path: "/this/is/the/path",
				QueryStringParameters: map[string]string{
					"q": "q1",
				},
			},
			response: events.APIGatewayProxyResponse{
				StatusCode: 200,
				Headers: map[string]string{
					"Content-Type": "text/plain",
				},
				Body: "/this/is/the/path?q=q1\nGET",
			},
		},
		{
			// body setup from POST
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := ioutil.ReadAll(r.Body)
				w.Write([]byte(body))
			}),
			request: events.APIGatewayProxyRequest{
				HTTPMethod:      "POST",
				Path:            "/test",
				Body:            "VGhpcyBpcyB0aGUgYm9keQo=",
				IsBase64Encoded: true,
				Headers:         map[string]string{},
			},
			response: events.APIGatewayProxyResponse{
				StatusCode: 200,
				Headers:    map[string]string{},
				Body:       "This is the body\n",
			},
		},
	}

	for i, tt := range tests {
		handler := apiGatewayHandler(tt.handler)

		response, err := handler(tt.request)
		if err != nil {
			if tt.err == nil {
				t.Errorf("%d: got %v, want no error", i, err)
			}
			continue
		} else if tt.err != nil {
			t.Errorf("%d: got no error, expected %v", i, tt.err)
			continue
		}

		if !reflect.DeepEqual(response, tt.response) {
			t.Errorf("%d: got:\n%s\nwant:\n%s", i, spew.Sprint(response), spew.Sprint(tt.response))
		}
	}
}
