# Veneur, in Docker

This folder holds the Docker resources for building [Veneur](https://github.com/stripe/veneur), and a sample systemd unit for running it. It lives in the Veneur repository, but currently clones from Github, so the state of the repository you build in is unrelated to the output.

## Building

The `--no-cache` argument may be necessary to fetch the latest commits from Github, unless you're building a specific version tag.

```
public-docker-images$ docker build --no-cache -t veneur:local -f Dockerfile-debian-sid .
```

For the Alpine Linux-based image:
```
public-docker-images$ docker build --no-cache -t veneur:local -f Dockerfile-alpine .
```

For a specific tag or branch:
```
public-docker-images$ docker build -t veneur:local --build-arg BUILD_REF='tag-or-branch' -f Dockerfile-debian-sid .
```

## Running

The example.yaml comes built-in with the docker image, but won't be usefully functional without supplying a few [configuration options](https://github.com/stripe/veneur#configuration-via-environment-variables) and run arguments.

In the foreground, with Datadog as a metrics sink, receiving StatsD packets on port 8126 (UDP) and responding to healthchecks over HTTP on port 8127:

```
$ docker run --rm -it \
    --name veneur-docker \
    -p 8126:8126/udp \
    -p 8127:8127/tcp \
    -e VENEUR_DATADOGAPIKEY="<your key>" \
    --net=host \
    veneur:local
```

`--net=host` is required for Veneur to listen on `localhost` if you want to be able to send packets to it. You can also configure Veneur to listen on `0.0.0.0` and send data to its Docker-assigned address.

`VENEUR_DATADOGAPIKEY` passes the API key to use when flushing to Datadog. You'll need to configure most sinks in some fashion or other; see the full list of [configuration keys](https://github.com/stripe/veneur#configuration) as well as [how to phrase them](https://github.com/stripe/veneur#configuration-via-environment-variables) as environment variables.

## Systemd

[veneur.service](https://github.com/stripe/veneur/tree/master/public-docker-images/veneur.service) can be installed as a systemd unit capable of managing Veneur as a docker container. It expects an environment configuration file in `/etc/default/veneur`; this should contain your settings, including API keys. You may also specify which Docker image to use here, via the environment variable `DOCKER_IMAGE` (default is `veneur`).

## Veneur-emit

`veneur-emit` is also included in the image, and can be run explicitly like so:
```
$ docker run --net=host -it veneur /veneur/veneur-emit -count 1 -name foo -hostport "127.0.0.1:8126"
```
