package apigatewayproxy

import (
	"io"
	"net/http"
	"os"
	"reflect"
	"strings"
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
		handler     http.Handler
		request     events.APIGatewayProxyRequest
		response    events.APIGatewayProxyResponse
		expectError bool
	}{
		{
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var buf [256]byte
				if _, err := r.Body.Read(buf[:]); err != io.EOF {
					t.Errorf("got %v, want %v", err, io.EOF)
				}
				evreq := Request(r.Context())
				if evreq == nil {
					t.Error("got nil, want request")
				}
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
				w.Write([]byte(r.Header.Get("Accept")))
				w.Write([]byte{0x0a})
				w.Write([]byte(r.Header.Get("Accept-Encoding")))
			}),
			request: events.APIGatewayProxyRequest{
				Path:       "/test",
				HTTPMethod: "GET",
				Headers: map[string]string{
					"Accept":          "*/*",
					"Accept-Encoding": "gzip",
				},
			},
			response: events.APIGatewayProxyResponse{
				StatusCode: 200,
				Headers:    map[string]string{},
				Body:       "*/*\ngzip",
			},
		},
		{
			// converts binary content to base 64
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/octet-stream")
				w.Write([]byte{0x0a, 0x0b, 0x0c, 0xff})
			}),
			request: events.APIGatewayProxyRequest{
				Path:       "/test",
				HTTPMethod: "GET",
				Headers:    map[string]string{},
			},
			response: events.APIGatewayProxyResponse{
				StatusCode: 200,
				Headers: map[string]string{
					"Content-Type": "application/octet-stream",
				},
				Body:            "CgsM/w==",
				IsBase64Encoded: true,
			},
		},
		{
			// converts content-encoded content to base 64
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/octet-stream")
				w.Header().Set("Content-Encoding", "whatever")
				w.Write([]byte{0x0a, 0x0b, 0x0c, 0xff})
			}),
			request: events.APIGatewayProxyRequest{
				Path:       "/test",
				HTTPMethod: "GET",
				Headers:    map[string]string{},
			},
			response: events.APIGatewayProxyResponse{
				StatusCode: 200,
				Headers: map[string]string{
					"Content-Type":     "application/octet-stream",
					"Content-Encoding": "whatever",
				},
				Body:            "CgsM/w==",
				IsBase64Encoded: true,
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
			// RequestURI set correctly from path and query
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("content-type", "text/plain")
				w.Write([]byte("RequestURI="))
				w.Write([]byte(r.RequestURI))
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
				Body: "RequestURI=/this/is/the/path?q=q1\nGET",
			},
		},
		{
			// body setup from POST
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				w.Write([]byte(body))
			}),
			request: events.APIGatewayProxyRequest{
				HTTPMethod:      "POST",
				Path:            "/test",
				Body:            "This is the body\n",
				IsBase64Encoded: false,
				Headers:         map[string]string{},
			},
			response: events.APIGatewayProxyResponse{
				StatusCode: 200,
				Headers:    map[string]string{},
				Body:       "This is the body\n",
			},
		},
		{
			// body setup from POST
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
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
		{
			// multiple request headers with same name
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				multi := r.Header["X-Multi"]
				body := strings.Join(multi, ",")
				_, _ = w.Write([]byte(body))
			}),
			request: events.APIGatewayProxyRequest{
				HTTPMethod:      "POST",
				Path:            "/test",
				Body:            "VGhpcyBpcyB0aGUgYm9keQo=",
				IsBase64Encoded: true,
				Headers:         map[string]string{},
				MultiValueHeaders: map[string][]string{
					"X-Multi": {"a", "b", "c"},
				},
			},
			response: events.APIGatewayProxyResponse{
				StatusCode: 200,
				Headers:    map[string]string{},
				Body:       "a,b,c",
			},
		},
		{
			// multiple query string values with same name
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body := r.URL.RawQuery
				_, _ = w.Write([]byte(body))
			}),
			request: events.APIGatewayProxyRequest{
				HTTPMethod: "GET",
				Path:       "/test",
				Headers:    map[string]string{},
				MultiValueQueryStringParameters: map[string][]string{
					"q": {"a", "b", "c"},
				},
			},
			response: events.APIGatewayProxyResponse{
				StatusCode: 200,
				Headers:    map[string]string{},
				Body:       "q=a&q=b&q=c",
			},
		},
		{
			// multiple response headers with same name
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header()["X-Multi"] = []string{"a", "b", "c"}
				body := "response"
				_, _ = w.Write([]byte(body))
			}),
			request: events.APIGatewayProxyRequest{
				HTTPMethod: "GET",
				Path:       "/test",
			},
			response: events.APIGatewayProxyResponse{
				StatusCode: 200,
				Headers:    map[string]string{},
				MultiValueHeaders: map[string][]string{
					"X-Multi": {"a", "b", "c"},
				},
				Body: "response",
			},
		},
		{
			request: events.APIGatewayProxyRequest{
				HTTPMethod:      "POST",
				Path:            "/test",
				Body:            "V#GhpcyBpcyB0aGUgYm9keQo=", // dodgy base 64
				IsBase64Encoded: true,
				Headers:         map[string]string{},
			},
			expectError: true,
		},
		{
			request: events.APIGatewayProxyRequest{
				HTTPMethod: "GET",
				Path:       ":\\test", // dodgy path
				Headers:    map[string]string{},
			},
			expectError: true,
		},
	}

	for i, tt := range tests {
		handler := apiGatewayHandler(tt.handler)

		response, err := handler(tt.request)
		if err != nil {
			if !tt.expectError {
				t.Errorf("%d: got %v, want no error", i, err)
			}
			continue
		} else if tt.expectError {
			t.Errorf("%d: got no error, expected error", i)
			continue
		}

		if !reflect.DeepEqual(response, tt.response) {
			t.Errorf("%d: got:\n%s\nwant:\n%s", i, spew.Sprint(response), spew.Sprint(tt.response))
		}
	}
}
