# apigatewayproxy [![GoDoc](https://godoc.org/github.com/jjeffery/apigatewayproxy?status.svg)](https://godoc.org/github.com/jjeffery/apigatewayproxy) [![License](http://img.shields.io/badge/license-MIT-green.svg?style=flat)](https://raw.githubusercontent.com/jjeffery/apigatewayproxy/master/LICENSE.md) [![Build Status](https://travis-ci.org/jjeffery/apigatewayproxy.svg?branch=master)](https://travis-ci.org/jjeffery/apigatewayproxy) [![Coverage Status](https://coveralls.io/repos/github/jjeffery/apigatewayproxy/badge.svg?branch=master)](https://coveralls.io/github/jjeffery/apigatewayproxy?branch=master) [![GoReportCard](https://goreportcard.com/badge/github.com/jjeffery/apigatewayproxy)](https://goreportcard.com/report/github.com/jjeffery/apigatewayproxy)

Package `apigatwayproxy` provides a way to process AWS API Gateway Proxy requests using a standard `http.Handler`.

This makes it simple to build a program that operates as a HTTP server when running outside an AWS Lambda container,
and runs as an AWS Lambda when running inside an AWS Lambda container.

[Read the package documentation for more information](https://godoc.org/github.com/jjeffery/apigatewayproxy).

## Licence

MIT

