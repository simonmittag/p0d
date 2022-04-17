[![Circleci Builds](https://circleci.com/gh/simonmittag/p0d.svg?style=shield)](https://circleci.com/gh/simonmittag/p0d)
[![Github Issues](https://img.shields.io/github/issues/simonmittag/p0d)](https://github.com/simonmittag/p0d/issues)
[![Github Activity](https://img.shields.io/github/commit-activity/m/simonmittag/p0d)](https://img.shields.io/github/commit-activity/m/simonmittag/p0d)  
[![Go Report](https://goreportcard.com/badge/github.com/simonmittag/p0d)](https://goreportcard.com/report/github.com/simonmittag/p0d)
[![Codeclimate Maintainability](https://api.codeclimate.com/v1/badges/06a7484f009ea48a3832/maintainability)](https://codeclimate.com/github/simonmittag/p0d/maintainability)
[![Codeclimate Test Coverage](https://api.codeclimate.com/v1/badges/06a7484f009ea48a3832/test_coverage)](https://codeclimate.com/github/simonmittag/p0d/test_coverage)
[![Go Version](https://img.shields.io/github/go-mod/go-version/simonmittag/p0d)](https://img.shields.io/github/go-mod/go-version/simonmittag/p0d)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![Version](https://img.shields.io/badge/version-0.1.8-orange)](https://github.com/simonmittag/p0d)

## What is p0d?
![](p0d.png)

p0d is a go based HTTP performance testing tool. It can be configured via cli or yml. p0d
runs HTTP/1.1 and HTTP/2 requests in parallel against your server and provides detailed
reports on the output. p0d is alpha grade software under active development.

## Up and running

### Golang
```bash
go get github.com/simonmittag/p0d && 
go install github.com/simonmittag/p0d/cmd/p0d && 
p0d -h
```

## Usage

### With cli args
```
λ p0d -h
  Usage of p0d:
  -C string
        load configuration from yml file
  -c int
        maximum amount of parallel TCP connections used (default 1)
  -d int
        time in seconds to run p0d (default 10)
  -t int
        amount of parallel execution threads (default 1)
  -u string
        url to use
  -v    print p0d version
```

### With a config file
`pod -c config.yml`

```
---
exec:
  mode: binary
  durationSeconds: 30
  threads: 128
  connections: 128
  logsampling: 0.1
req:
  method: GET
  url: http://localhost:60083/mse6/get
  headers:
    - Accept-Encoding: "identity"
res:
  code: 200
```

## Contributions

The j8a team welcomes all [contributors](https://github.com/simonmittag/p0d/blob/master/CONTRIBUTING.md). Everyone
interacting with the project's codebase, issue trackers, chat rooms and mailing lists is expected to follow
the [code of conduct](https://github.com/simonmittag/p0d/blob/master/CODE_OF_CONDUCT.md)