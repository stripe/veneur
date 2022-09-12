# Veneur, in Docker

This folder holds the Docker resources for building [Veneur](https://github.com/stripe/veneur), and a sample systemd unit for running it.

## Building

You should run these commands from the project root.

For the Debian-based image:

```
docker buildx build --platform=linux/amd64,linux/arm64 --build-arg=VERSION=$(git rev-parse HEAD) --no-cache -t veneur:local -f public-docker-images/Dockerfile-debian-sid --pull --push .
```

For the Alpine Linux-based image:

```
docker buildx build --platform=linux/amd64,linux/arm64 --build-arg=VERSION=$(git rev-parse HEAD) --no-cache -t veneur:local -f public-docker-images/Dockerfile-alpine --pull --push .
```

For both cases you could remove ```--platform` arugment if you just plan build for the host architechture.

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
