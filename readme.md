# `panascais/mongodb`

[![GitHub Workflow Status](https://img.shields.io/github/actions/workflow/status/panascais-docker/mongodb/main.yml?branch=master&style=flat-square)](https://github.com/panascais-docker/mongodb/actions?query=workflow%3Amain)
[![Docker Pulls](https://img.shields.io/docker/pulls/panascais/mongodb.svg?style=flat-square)](https://hub.docker.com/r/panascais/mongodb)
[![Docker Stars](https://img.shields.io/docker/stars/panascais/mongodb.svg?style=flat-square)](https://hub.docker.com/r/panascais/mongodb)
[![Docker Image Size](https://img.shields.io/docker/image-size/panascais/mongodb.svg?style=flat-square)](https://hub.docker.com/r/panascais/mongodb)
[![License](https://img.shields.io/github/license/panascais-docker/mongodb.svg?style=flat-square)](https://hub.docker.com/r/panascais/mongodb)

Small MongoDB images built from the official [`mongodb/mongodb-community-server`](https://hub.docker.com/r/mongodb/mongodb-community-server) slim images. They keep upstream's Red Hat UBI micro filesystem, strip `mongod` and replace the entrypoint with a single Go binary.

| **Tag:**     | **Command:**                          | **MongoDB Version:** | **Variants:**         | **Flavors:**                     |
| ------------ | ------------------------------------- | -------------------- | --------------------- | -------------------------------- |
| `latest`     | `docker pull panascais/mongodb`       | `9.0.x`              | ubi8, ubi9, **ubi10** | standalone, `-replica`, `-cluster` |
| `9.0`, `9`   | `docker pull panascais/mongodb:9.0`   | `9.0.x`              | ubi8, ubi9, **ubi10** | standalone, `-replica`, `-cluster` |
| `8.3`, `8`   | `docker pull panascais/mongodb:8.3`   | `8.3.x`              | ubi8, ubi9, **ubi10** | standalone, `-replica`, `-cluster` |
| `8.2`        | `docker pull panascais/mongodb:8.2`   | `8.2.x`              | ubi8, **ubi9**        | standalone, `-replica`, `-cluster` |
| `8.0`        | `docker pull panascais/mongodb:8.0`   | `8.0.x`              | ubi8, ubi9, **ubi10** | standalone, `-replica`, `-cluster` |
| `7.0`, `7`   | `docker pull panascais/mongodb:7.0`   | `7.0.x`              | ubi8, ubi9, **ubi10** | standalone, `-replica`, `-cluster` |
| `6.0`, `6`   | `docker pull panascais/mongodb:6.0`   | `6.0.x`              | ubi8, **ubi9**        | standalone, `-replica`, `-cluster` |
| `5.0`, `5`   | `docker pull panascais/mongodb:5.0`   | `5.0.x`              | **ubi8**              | standalone, `-replica`, `-cluster` |
| `4.4`, `4`   | `docker pull panascais/mongodb:4.4`   | `4.4.x`              | **ubi8**              | standalone, `-replica`, `-cluster` |

Tags are built as `<version>[-<variant>][-<flavor>]`, and every combination exists:

| **Part:**   | **Values:**                                       | **When left out:**                          |
| ----------- | ------------------------------------------------- | ------------------------------------------- |
| version     | `latest`, a major `9`, a line `9.0`, a patch `9.0.2` | always required                          |
| variant     | `-ubi8`, `-ubi9`, `-ubi10`                        | the newest ubi the line ships, bold above   |
| flavor      | `-standalone`, `-replica`, `-cluster`             | a standalone `mongod`                       |

For example `9` and `9-standalone` are a standalone `mongod` on ubi10, `9-replica` a replica set on ubi10, `9.0.2-ubi9-cluster` a cluster on ubi9 and `latest-ubi8-replica` a replica set on ubi8. ubi10 needs an x86-64-v3 CPU (AVX2) on amd64, so use a `-ubi9` tag on older amd64 machines or under emulation.

## Usage

```sh
docker run -d -p 27017:27017 \
    -e MONGODB_ROOT_USERNAME=root \
    -e MONGODB_ROOT_PASSWORD=secret \
    panascais/mongodb
```

- `MONGODB_ROOT_USERNAME` and `MONGODB_ROOT_PASSWORD`, or `*_FILE` pointing to a secret, create a root user on first start and enable `--auth`. Without them mongod runs without auth.
- Arguments starting with `-` go to `mongod`, for example `docker run panascais/mongodb --port 27018`. `--bind_ip_all` is added unless you pass `--bind_ip`, `--bind_ip_all` or a config file.
- The image has a `HEALTHCHECK`. With a root user configured it only turns healthy once that user can log in.
- mongod starts with `--networkMessageCompressors zstd,snappy`, `--timeStampFormat iso8601-utc`, `--wiredTigerJournalCompressor zstd` and `--wiredTigerCollectionBlockCompressor zstd`. A new data directory also gets `--directoryperdb` and `--wiredTigerDirectoryForIndexes`, an existing one keeps the layout it was created with. Passing a flag yourself overrides its default, and a config file replaces all of them.

Compared to the upstream image there is no `mongosh`, no `mongos`, no `/docker-entrypoint-initdb.d` and none of the `MONGODB_INITDB_*` variables. Connect with `mongosh` from your host or another container.

## Replica set and cluster for CI

Every tag also exists with a `-replica` or `-cluster` suffix, for example `9.0-replica`, `9.0.2-ubi10-cluster` or `latest-replica`. They preconfigure a topology inside the one container, so they are meant for CI and local development, not production.

- `-replica` runs a single node replica set named `rs0` (`MONGODB_REPLICA_SET`), which is enough for transactions and change streams.
- `-cluster` runs `mongos` on 27017 with a single node config server on 27018 on the loopback interface and a single node shard named `shard` on 27019. The shard listens on all interfaces, so tests can publish 27019 and connect to it with `directConnection=true`. With a root user configured, the same user is also created on the shard itself.

Both use the same mongod defaults as the plain image plus `periodicNoopIntervalSecs=1`, so change streams on an idle deployment advance within a second. Both accept `MONGODB_ROOT_USERNAME` and `MONGODB_ROOT_PASSWORD` like the plain image, `MONGODB_PORT` to move the listening port, and only turn healthy once setup has finished.

Arguments after `--` go to mongod. In the cluster they go to the config server and the shard, never to `mongos`. Because they replace the image's `CMD`, name the flavor first:

```sh
docker run -d -p 27017:27017 panascais/mongodb:9.0-replica \
    replica -- --setParameter enableTestCommands=1 --wiredTigerCacheSizeGB 0.25 --oplogSize 64
docker run -d -p 27017:27017 -p 27019:27019 panascais/mongodb:9.0-cluster \
    cluster -- --setParameter enableTestCommands=1 --wiredTigerCacheSizeGB 0.25 --oplogSize 64
```

Passing a default yourself overrides it, and `--setParameter` defaults are matched by parameter name, so `--setParameter enableTestCommands=1` keeps `periodicNoopIntervalSecs=1`. Leave `--port`, `--replSet`, `--dbpath` and `--bind_ip` to the entrypoint.

The cluster works from anywhere, because clients only talk to `mongos`. For the replica set, drivers reconnect to the host the member advertises, so `MONGODB_REPLICA_HOST` has to be the name your clients reach the container by:

| **Clients run:**                                        | **Container:**                                  | **Connection string:**                      |
| ------------------------------------------------------- | ----------------------------------------------- | ------------------------------------------- |
| on the host or CI runner, with `-p 27017:27017`         | defaults                                        | `mongodb://localhost:27017/?replicaSet=rs0` |
| in a container on the same network, compose or CI jobs  | `MONGODB_REPLICA_HOST=mongodb` (service name)   | `mongodb://mongodb:27017/?replicaSet=rs0`   |
| on the host, published on another port                  | `MONGODB_PORT=37017` with `-p 37017:37017`      | `mongodb://localhost:37017/?replicaSet=rs0` |

Clients that need to connect from both sides at once can add `directConnection=true` instead.

## Kernel rseq regression

Linux 6.19 through 7.0.13 crash mongod while tcmalloc uses per-CPU caches, and upstream's entrypoint refuses to start on any kernel from 6.19 on. On affected kernels this image prints a warning and sets `GLIBC_TUNABLES=glibc.pthread.rseq=1`, so tcmalloc falls back to per-thread caches. Elsewhere it sets `glibc.pthread.rseq=0`. A `glibc.pthread.rseq` value you set yourself always wins.

## Build

The repository holds two Go programs in one module:

- `entrypoint/` is the image entrypoint, built into `/usr/local/bin/mongodb-entrypoint` by the `Dockerfile`.
- `scripts/` builds the images and keeps the upstream pins in `configuration/` current.

```sh
go run ./scripts build 9.0
```

This builds every variant and flavor of a line for the local architecture and waits for each image to become healthy, once without and once with a root user. `configuration/tags.json` and `configuration/digests.json` pin the upstream images by digest, `configuration/builders.json` pins the `golang` and `alpine` images the `Dockerfile` builds with, and `go run ./scripts update` refreshes them. A new builder digest rebuilds every line.

## Contributors

- Silas Rech [(silas@panascais.net)](mailto:silas@panascais.net)
- Maximilian Schagginger [(max@panascais.net)](mailto:max@panascais.net)

## Contributing

Interested in contributing to **MongoDB**? Contributions are welcome, and are accepted via pull requests. Please [review these guidelines](contributing.md) before submitting any pull requests.

## License

Code licensed under [MIT](license), documentation under [CC BY 3.0](https://creativecommons.org/licenses/by/3.0/). MongoDB itself is licensed under the [SSPL](https://www.mongodb.com/legal/licensing/server-side-public-license).
