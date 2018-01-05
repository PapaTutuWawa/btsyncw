# btsyncw
A simple wrapper for executing my modified version of the Resilio docker image.

## Prerequesites
- docker client: ```$ go get github.com/docker/docker/client```
- docker API: ```$ go get github.com/docker/docker/api```
- net context: ```$ go get golang.org/x/net/context```

## Build
```
$ go build -o btsyncw main.go
```

## Usage
```
$ btsyncw <config>
```

(See test.json for an example)

