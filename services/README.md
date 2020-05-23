# Shuffle services

This directory contains Go source code for these services:

* Backend server
* Orborus
* Worker
* Webhook endpoint

# Build

The all-in-one executable :

```shell script
$ go build -o shuffle cmd/shuffle/main.go
```

# Usage

Each subcommand (`server, orborus, worker and webhook`) can display its help with

```shell script
$ ./shuffle help <subcommand>
```
The arguments can be passed with environment variables prefixed with `SHUFFLE_`. 

The subcommand can be passed with the `SHUFFLE_COMMAND` variable. 

# Docker

To build the Docker image:

```shell script
$ docker build .
```

To run the backend server:

```shell script
$ docker run -e SHUFFLE_COMMAND=server <IMAGE ID>
```


