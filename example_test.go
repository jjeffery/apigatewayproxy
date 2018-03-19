package apigatewayproxy_test

import (
	"fmt"
	"net/http"

	"github.com/jjeffery/apigatewayproxy"
)

func Example() {
	// define a simple HTTP handler
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello world\n"))
	})

	if apigatewayproxy.IsLambda() {
		// this process is running in an AWS Lambda container
		apigatewayproxy.Start(h)
	} else {
		// run as a conventional HTTP server
		http.ListenAndServe(":8080", h)
	}
}

func ExampleRequest() {
	// define a HTTP handler that knows whether it is
	// running as an AWS lambda
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pr := apigatewayproxy.Request(r.Context())
		var msg string
		if pr == nil {
			msg = "I am not running in an AWS Lambda container"
		} else {
			msg = fmt.Sprintf("I am running in an AWS Lambda under account %s", pr.RequestContext.AccountID)
		}
		w.Write([]byte(msg))
	})

	if apigatewayproxy.IsLambda() {
		// this process is running in an AWS Lambda container
		apigatewayproxy.Start(h)
	} else {
		// run as a conventional HTTP server
		http.ListenAndServe(":8080", h)
	}
}
