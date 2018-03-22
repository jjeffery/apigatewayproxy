// Package apigatewayproxy provides a way to process
// AWS API Gateway Proxy requests using a standard HTTP handler.
// This makes it simple to build a program that operates as a
// HTTP server when run normally, and runs as an AWS lambda
// when running in an AWS lambda container.
package apigatewayproxy

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/golang/gddo/httputil"
	"github.com/jjeffery/errors"
)

type ctxKey int

const (
	ctxKeyEventContext ctxKey = 1
)

var allowCompression bool

var (
	// RequestReceived is called when a request is received from Lambda
	RequestReceived = func(request *events.APIGatewayProxyRequest) {}

	// BeforeSendResponse is called prior to returning the response to Lambda
	BeforeSendResponse = func(request *events.APIGatewayProxyRequest, response *events.APIGatewayProxyResponse) {}
)

func init() {
	if _, ok := os.LookupEnv("NO_COMPRESSION"); !ok {
		allowCompression = true
	}
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
			header:            make(http.Header),
			preferredEncoding: preferredEncoding(r),
		}
		h.ServeHTTP(&w, r)
		w.finished()
		BeforeSendResponse(&request, &w.response)
		return w.response, w.err
	}
}

func preferredEncoding(r *http.Request) string {
	return httputil.NegotiateContentEncoding(r, []string{"gzip"})
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

	r, err := http.NewRequest(request.HTTPMethod, u.String(), body)
	if err != nil {
		return nil, errors.Wrap(err, "cannot create HTTP request")
	}

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

// compressedContentTypes is a list of mime types that already
// contain compressed content, so they should not be gzipped.
var compressedContentTypes = []string{
	"image/gif",
	"image/jpeg",
	"image/png",
}

func isCompressedContentType(contentType string) bool {
	for _, ct := range compressedContentTypes {
		if strings.HasPrefix(contentType, ct) {
			return true
		}
	}
	return false
}

func (w *responseWriter) finished() {
	// write the header if it has not already been written
	if !w.headersWritten {
		w.WriteHeader(http.StatusOK)
	}

	// compression is easy to put here because the response content
	// is in a buffer and we know the lengths, so it's included

	var shouldCompress bool
	if allowCompression && w.preferredEncoding == "gzip" {
		contentEncoding := w.response.Headers["Content-Encoding"]
		if contentEncoding == "" || contentEncoding == "identity" {
			contentType := w.response.Headers["Content-Type"]
			shouldCompress = !isCompressedContentType(contentType) && w.body.Len() > 256
		}
	}

	if shouldCompress {
		var buf bytes.Buffer

		// Error handling is unusual here because if something fails during
		// compression, we just continue with the uncompressed response.
		// So instead of the usual "if err != nil", we have a succession of
		// "if err == nil" tests.
		writer, err := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
		if err == nil {
			reader := bytes.NewReader(w.body.Bytes())
			if _, err := io.Copy(writer, reader); err == nil {
				// don't bother if the compressed content is larger than the
				// uncompressed content
				if buf.Len() < w.body.Len() {
					w.body = buf
					w.response.Headers["Content-Encoding"] = "gzip"
					w.response.Headers["Content-Length"] = strconv.Itoa(w.body.Len())
					vary := w.response.Headers["Vary"]
					if vary == "" {
						w.response.Headers["Vary"] = "Accept-Encoding"
					} else {
						varyLower := strings.ToLower(vary)
						if !strings.Contains(varyLower, "accept-encoding") && !strings.Contains(varyLower, "*") {
							w.response.Headers["Vary"] = vary + ", Accept-Encoding"
						}
					}
				}
			}
		}
	}

	// Regardless of the content type or the content encoding, if the body is
	// valid UTF8, return it as a string. The call to utf8.Valid will return
	// false quickly for most binary payloads, including compressed payloads.
	//
	// Note that if the body starts with UTF8 byte order marks (0xef, 0xbb, 0xbf), it will
	// be base64 encoded. This is the correct behaviour, because BOMs in the middle of
	// a UTF8 string are not valid, and the body will be part of a larger, JSON string.
	b := w.body.Bytes()
	if shouldEncode(b) {
		w.response.Body = base64.StdEncoding.EncodeToString(b)
		w.response.IsBase64Encoded = true
	} else {
		w.response.Body = string(b)
		w.response.IsBase64Encoded = false
	}
}

func shouldEncode(buf []byte) bool {
	for _, b := range buf {
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
