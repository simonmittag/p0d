![](p0d.png)

[![Circleci Builds](https://circleci.com/gh/simonmittag/p0d.svg?style=shield)](https://circleci.com/gh/simonmittag/p0d)
[![Github Issues](https://img.shields.io/github/issues/simonmittag/p0d)](https://github.com/simonmittag/p0d/issues)
[![Github Activity](https://img.shields.io/github/commit-activity/m/simonmittag/p0d)](https://img.shields.io/github/commit-activity/m/simonmittag/p0d)  
[![Codeclimate Maintainability](https://api.codeclimate.com/v1/badges/06a7484f009ea48a3832/maintainability)](https://codeclimate.com/github/simonmittag/p0d/maintainability)
[![Codeclimate Test Coverage](https://api.codeclimate.com/v1/badges/06a7484f009ea48a3832/test_coverage)](https://codeclimate.com/github/simonmittag/p0d/test_coverage)
[![Go Version](https://img.shields.io/github/go-mod/go-version/simonmittag/p0d)](https://img.shields.io/github/go-mod/go-version/simonmittag/p0d)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)

p0d is a go based HTTP performance testing tool. It can be configured via cli or yml. p0d
runs HTTP/1.1 and HTTP/2 requests in parallel against your server and provides detailed
reports on the output.

## Up and running

### Golang
```bash
go get github.com/simonmittag/p0d && 
go install github.com/simonmittag/p0d/cmd/p0d && 
p0d -h
```

## Usage
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
