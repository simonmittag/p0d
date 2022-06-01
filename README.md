[![Circleci Builds](https://circleci.com/gh/simonmittag/p0d.svg?style=shield)](https://circleci.com/gh/simonmittag/p0d)
[![Github Issues](https://img.shields.io/github/issues/simonmittag/p0d)](https://github.com/simonmittag/p0d/issues)
[![Github Activity](https://img.shields.io/github/commit-activity/m/simonmittag/p0d)](https://img.shields.io/github/commit-activity/m/simonmittag/p0d)  
[![Go Report](https://goreportcard.com/badge/github.com/simonmittag/p0d)](https://goreportcard.com/report/github.com/simonmittag/p0d)
[![Codeclimate Maintainability](https://api.codeclimate.com/v1/badges/06a7484f009ea48a3832/maintainability)](https://codeclimate.com/github/simonmittag/p0d/maintainability)
[![Codeclimate Test Coverage](https://api.codeclimate.com/v1/badges/06a7484f009ea48a3832/test_coverage)](https://codeclimate.com/github/simonmittag/p0d/test_coverage)
[![Go Version](https://img.shields.io/github/go-mod/go-version/simonmittag/p0d)](https://img.shields.io/github/go-mod/go-version/simonmittag/p0d)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![Version](https://img.shields.io/badge/version-0.3.0-orange)](https://github.com/simonmittag/p0d)

## What is p0d?
![](p0d_80.png)

p0d is a cli based HTTP performance testing tool for Linux and Darwin operating systems. Features include support
for HTTP/1.1, HTTP/2, file attachments as multipart/form-data, and TLS. p0d provides realtime updates
on stdout, including errors within test runs. p0d is beta grade software under active development, `v0.3.0` was
released 27/05/22.

## Up and running

### Golang
```bash
go install github.com/simonmittag/p0d/cmd/p0d@latest && p0d -h
```

## Usage Samples

Run for 30 seconds with 10 concurrent connections against local server
```
λ p0d -d 30 -c 10 http://localhost:8080/path
```

Run in http/2 mode against local server and save output to `log.json`
```
λ p0d -H 2 -O log.json http://localhost:8080/path
```

Run with config file
```
λ p0d -C config_get.yml
```

![](bash.gif)

### Cli args
```
λ p0d v0.2.9
 usage: p0d [-f flag] [URL]

 flags:
  -C string
        load configuration from yml file
  -H string
        http version to use. Values are 1.1 and 2 (which works only with TLS URLs). Defaults to 1.1 (default "1.1")
  -O string
        save detailed JSON output to file
  -c int
        maximum amount of concurrent TCP connections used (default 1)
  -d int
        time in seconds to run p0d (default 10)
  -h    
        print usage instructions
  -s 
        skip internet speed test, i.e. for local targets
  -v    
        print version
```

### Config file reference

```
---
exec:
  mode: binary
  durationSeconds: 10
  concurrency: 1
  logsampling: 1
  skipInetTest: true
req:
  method: POST
  url: http://localhost:8080/path
  headers:
    - Accept-Encoding: "identity"
  body: '
   { "your": "body" }
  '
res:
  code: 200
```

#### exec.mode
`binary` or `decimal` for MiB or MB units in reporting

#### exec.durationsSeconds
run pod for `n` seconds. Defaults to `10`

#### exec.dialTimeoutSeconds
give up connecting to upstream resource after `n` seconds. Defaults to `3`

#### exec.concurrency
use a pool of maximum `n` concurrent TCP connections. Defaults to `1`. Make sure your OS supports
sufficient open file descriptors before settings this to a very high value. 

#### exec.spacingMillis
artificial spacing in milliseconds, introduced before sending each request. Defaults to `0`

#### exec.httpVersion
preferred http version. Allowable values are `1.1`. and `2`. Defaults to `1.1`. Please note that HTTP/2 is only
supported using TLS. Http version is negotiated, not absolute and HTTP/2 may fall back to HTTP/1.1

#### exec.logsampling
ratio between `0.0` and `1.0` of requests to keep when saving results to disk with `-O`

#### exec.skipInetTest
skips the general internet speed test. Note this is not targetting your URL but the speedtest.net network.

#### req.method
http request method, usually one of `GET`, `PUT`, `POST`, or `DELETE`

#### req.url
upstream resource url. Must be supplied.

#### req.headers
list of headers to include in the request. use this to inject i.e. authentication

#### res.code
the expected http resonse code. if not matched, request counts as failed in test summary. Defaults to `200`

## Contributions

The p0d team welcomes all [contributors](https://github.com/simonmittag/p0d/blob/master/CONTRIBUTING.md). Everyone
interacting with the project's codebase, issue trackers, chat rooms and mailing lists is expected to follow
the [code of conduct](https://github.com/simonmittag/p0d/blob/master/CODE_OF_CONDUCT.md)