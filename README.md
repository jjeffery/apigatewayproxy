# apigatewayproxy 
[![Go Reference](https://pkg.go.dev/badge/github.com/jjeffery/apigatewayproxy.svg)](https://pkg.go.dev/github.com/jjeffery/apigatewayproxy)
[![Build status](https://ci.appveyor.com/api/projects/status/tcwg22wmapxaxuhn?svg=true)](https://ci.appveyor.com/project/jjeffery/apigatewayproxy)
[![License](http://img.shields.io/badge/license-MIT-green.svg?style=flat)](https://raw.githubusercontent.com/jjeffery/apigatewayproxy/master/LICENSE.md) 
[![GoReportCard](https://goreportcard.com/badge/github.com/jjeffery/apigatewayproxy)](https://goreportcard.com/report/github.com/jjeffery/apigatewayproxy)

Package `apigatwayproxy` provides a way to process AWS API Gateway Proxy requests using a standard `http.Handler`.

This makes it simple to build a program that operates as a HTTP server when running outside an AWS Lambda container,
and runs as an AWS Lambda when running inside an AWS Lambda container.

[Read the package documentation for more information](https://pkg.go.dev/github.com/jjeffery/apigatewayproxy).
