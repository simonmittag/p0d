# p0d
![](p0d.png)

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
