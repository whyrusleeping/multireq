# multireq
A small program to make one http request to multiple targets, and return the first one that comes back.

## Usage
```
$ multireq :7777 http://localhost:8000 http://localhost:9000
```

This listens on port 7777 and redirects incoming requests to both localhost:8000 and localhost:9000. The first of those to return is returned to the client, the other is cancelled.

## Installation
```
$ go get github.com/whyrusleeping/multireq
```
