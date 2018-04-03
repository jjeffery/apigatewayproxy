// Package apigatewayproxy provides a way to process
// AWS API Gateway Proxy requests using a standard HTTP handler.
// This makes it simple to build a program that operates as a
// HTTP server when run normally, and runs as an AWS lambda
// when running in an AWS lambda container.
package apigatewayproxy

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/jjeffery/errors"
)

type ctxKey int

const (
	ctxKeyEventContext ctxKey = 1
)

// Callback functions that can be overridden.
var (
	// RequestReceived is called when a request is received from Lambda. Useful for logging.
	// The default implementation does nothing.
	RequestReceived func(request *events.APIGatewayProxyRequest)

	// SendingResponse is called just prior to returning the response to Lambda. Useful for logging.
	// The default implementation does nothing.
	SendingResponse func(request *events.APIGatewayProxyRequest, response *events.APIGatewayProxyResponse)

	// ShouldEncodeBody is called to determine if the body should be base64-encoded.
	// The default implementation returns true if the response has a Content-Encoding header,
	// or if body contains bytes outside the range [0x09, 0x7f].
	ShouldEncodeBody func(response *events.APIGatewayProxyResponse, body []byte) bool
)

func init() {
	RequestReceived = func(request *events.APIGatewayProxyRequest) {}
	SendingResponse = func(request *events.APIGatewayProxyRequest, response *events.APIGatewayProxyResponse) {}
	ShouldEncodeBody = shouldEncodeBody
}

// IsLambda returns true if the current process is operating
// in an AWS Lambda container. It determines this by checking
// for the presence of the "_LAMBDA_SERVER_PORT" environment
// variable.
func IsLambda() bool {
	port := os.Getenv("_LAMBDA_SERVER_PORT")
	return port != ""
}

// Start starts handling AWS Lambda API Gateway proxy requests by passing
// each request to the HTTP hander function.
func Start(h http.Handler) {
	lambda.Start(apiGatewayHandler(h))
}

// Request returns a pointer to the API Gateway proxy request, or nil if the
// current context is not associated with an API Gateway proxy lambda.
func Request(ctx context.Context) *events.APIGatewayProxyRequest {
	request, _ := ctx.Value(ctxKeyEventContext).(*events.APIGatewayProxyRequest)
	return request
}

func apiGatewayHandler(h http.Handler) func(request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	return func(request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
		RequestReceived(&request)
		r, err := newRequest(&request)
		if err != nil {
			return events.APIGatewayProxyResponse{}, err
		}
		w := responseWriter{
			header: make(http.Header),
		}
		h.ServeHTTP(&w, r)
		w.finished()
		SendingResponse(&request, &w.response)
		return w.response, w.err
	}
}

type emptyReader struct{}

func (er emptyReader) Read(b []byte) (int, error) {
	return 0, io.EOF
}

func newRequest(request *events.APIGatewayProxyRequest) (*http.Request, error) {
	u, err := url.Parse(request.Path)
	if err != nil {
		return nil, errors.Wrap(err, "cannot parse request path").With("path", request.Path)
	}
	q := u.Query()
	for k, v := range request.QueryStringParameters {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()

	var body io.Reader
	{
		if request.Body == "" {
			// empty body
			body = emptyReader{}
		} else if request.IsBase64Encoded {
			b, err := base64.StdEncoding.DecodeString(request.Body)
			if err != nil {
				return nil, errors.Wrap(err, "cannot decode base64 body")
			}
			body = bytes.NewBuffer(b)
		} else {
			body = strings.NewReader(request.Body)
		}
	}

	requestURI := u.String()
	r, err := http.NewRequest(request.HTTPMethod, requestURI, body)
	if err != nil {
		return nil, errors.Wrap(err, "cannot create HTTP request")
	}
	// http.NewRequest does not set the RequestURI field
	r.RequestURI = requestURI

	for k, v := range request.Headers {
		r.Header.Set(k, v)
	}

	// add the request event to the request context so the HTTP handler
	// can access it if it wants
	ctx := context.WithValue(r.Context(), ctxKeyEventContext, request)
	r = r.WithContext(ctx)

	return r, nil
}

type responseWriter struct {
	preferredEncoding string
	response          events.APIGatewayProxyResponse
	body              bytes.Buffer
	header            http.Header
	headersWritten    bool
	err               error
}

func (w *responseWriter) Header() http.Header {
	return w.header
}

func (w *responseWriter) Write(b []byte) (int, error) {
	if !w.headersWritten {
		w.WriteHeader(http.StatusOK)
	}
	return w.body.Write(b)
}

func (w *responseWriter) WriteHeader(status int) {
	if w.headersWritten {
		return
	}
	w.response.StatusCode = status
	w.response.Headers = make(map[string]string)
	for k, vv := range w.header {
		for _, v := range vv {
			w.response.Headers[k] = v
		}
	}
	w.headersWritten = true
}

func (w *responseWriter) finished() {
	// write the header if it has not already been written
	w.WriteHeader(http.StatusOK)

	// Regardless of the content type or the content encoding, if the body is
	// valid UTF8, return it as a string. The call to utf8.Valid will return
	// false quickly for most binary payloads, including compressed payloads.
	//
	// Note that if the body starts with UTF8 byte order marks (0xef, 0xbb, 0xbf), it will
	// be base64 encoded. This is the correct behaviour, because BOMs in the middle of
	// a UTF8 string are not valid, and the body will be part of a larger, JSON string.
	b := w.body.Bytes()
	if ShouldEncodeBody(&w.response, b) {
		w.response.Body = base64.StdEncoding.EncodeToString(b)
		w.response.IsBase64Encoded = true
	} else {
		w.response.Body = string(b)
		w.response.IsBase64Encoded = false
	}
}

// shouldEncodeBody is the default implementation for ShouldEncodeBody
func shouldEncodeBody(response *events.APIGatewayProxyResponse, body []byte) bool {
	if contentEncoding := response.Headers["Content-Encoding"]; contentEncoding != "" && contentEncoding != "identity" {
		return true
	}
	for _, b := range body {
		switch b {
		case '\t', '\r', '\n':
			continue
		}
		if b < 0x20 || b > 0x7f {
			return true
		}
	}
	return false
}
