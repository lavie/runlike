    "See this docker container? I wish I could run another one just like it,
    but I'll be damned if I'm going to type all those command-line switches manually!"

This is what `runlike` does. You give it a docker container, it outputs the command line necessary to run another one just like it, along with all those pesky options (ports, links, volumes, ...). It's a real time saver for those that normally deploy their docker containers via some CM tool like Ansible/Chef and then find themselves needing to manually re-run some container.

**Notice:** This repo has been **renamed**. Used to be called `assaflavie/runlike`. Sorry for the inconvenience.

# Usage

    runlike <container-name>

This prints out what you need to run to get a similar container. You can do `$(runlike container-name)` to simply execute its output in one step.

`-p` breaks the command line down to nice, pretty lines. For example:

    $ runlike -p redis

    docker run \
        --name=redis \
        -e "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin" \
        -e "REDIS_VERSION=2.8.9" \
        -e "REDIS_DOWNLOAD_URL=http://download.redis.io/releases/redis-2.8.9.tar.gz" \
        -e "REDIS_DOWNLOAD_SHA1=003ccdc175816e0a751919cf508f1318e54aac1e" \
        -p 0.0.0.0:6379:6379/tcp \
        --detach=true \
        myrepo/redis:7860c450dbee9878d5215595b390b9be8fa94c89 \
        redis-server --slaveof 172.31.17.84 6379

## Run without installing

`runlike` is packaged as a Docker image: [assaflavie/runlike](https://hub.docker.com/r/assaflavie/runlike/). 

```
docker run --rm -v /var/run/docker.sock:/var/run/docker.sock \
    assaflavie/runlike YOUR-CONTAINER
```

You can also run it with an alias by adding the following to your `~/.profile` or `~/.bashrc`:

```
alias runlike="docker run --rm -v /var/run/docker.sock:/var/run/docker.sock assaflavie/runlike"
```

Now runlike should be available as a local command:

```
runlike YOUR-CONTAINER
```

# Install

    $ pip install runlike

# Status

This is very much a work in progress. Many `docker run` options aren't yet supported, but the most commonly used ones are. Feel free to send pull requests if you add any or if you happen to fix any (of the many) bugs this package undoubtedly has.

Probably **shouldn't use this in production** yet. If you do, double check that it's actually running what you want it to run.

[![Build Status](https://travis-ci.org/lavie/runlike.svg?branch=master)](https://travis-ci.org/lavie/runlike)

## Supported Run Options

```
      --add-host list                  Add a custom host-to-IP mapping
                                       (host:ip)
      --cap-add list                   Add Linux capabilities
      --cap-drop list                  Drop Linux capabilities
                                       (0-3, 0,1)
  -d, --detach                         Run container in background and
                                       print container ID
      --device list                    Add a host device to the container
      --dns list                       Set custom DNS servers
  -e, --env list                       Set environment variables
      --expose list                    Expose a port or a range of ports
  -h, --hostname string                Container host name
      --mac-address string             Container MAC address (e.g.,
                                       92:d0:c6:0a:29:33)
  -l, --label list                     Set meta data on a container
      --log-driver string              Logging driver for the container
      --log-opt list                   Log driver options
      --link list                      Add link to another container
      --name string                    Assign a name to the container
      --network string                 Connect a container to a network
                                       (default "default")
      --privileged                     Give extended privileges to this
                                       container
  -p, --publish list                   Publish a container's port(s) to
                                       the host
      --restart string                 Restart policy to apply when a
                                       container exits (default "no")
  -t, --tty                            Allocate a pseudo-TTY
  -u, --user string                    Username or UID (format:
                                       <name|uid>[:<group|gid>])
  -v, --volume list                    Bind mount a volume
      --volumes-from list              Mount volumes from the specified
                                       container(s)

```

## Not Yet Supported Run Options (PRs are most welcome!)

```

  -a, --attach list                    Attach to STDIN, STDOUT or STDERR
      --blkio-weight uint16            Block IO (relative weight),
                                       between 10 and 1000, or 0 to
                                       disable (default 0)
      --blkio-weight-device list       Block IO weight (relative device
                                       weight) (default [])

      --cgroup-parent string           Optional parent cgroup for the
                                       container
      --cidfile string                 Write the container ID to the file
      --cpu-count int                  CPU count (Windows only)
      --cpu-percent int                CPU percent (Windows only)
      --cpu-period int                 Limit CPU CFS (Completely Fair
                                       Scheduler) period
      --cpu-quota int                  Limit CPU CFS (Completely Fair
                                       Scheduler) quota
      --cpu-rt-period int              Limit CPU real-time period in
                                       microseconds
      --cpu-rt-runtime int             Limit CPU real-time runtime in
                                       microseconds
  -c, --cpu-shares int                 CPU shares (relative weight)
      --cpus decimal                   Number of CPUs
      --cpuset-cpus string             CPUs in which to allow execution
                                       (0-3, 0,1)
      --cpuset-mems string             MEMs in which to allow execution

      --detach-keys string             Override the key sequence for
                                       detaching a container
      --device-cgroup-rule list        Add a rule to the cgroup allowed
                                       devices list
      --device-read-bps list           Limit read rate (bytes per second)
                                       from a device (default [])
      --device-read-iops list          Limit read rate (IO per second)
                                       from a device (default [])
      --device-write-bps list          Limit write rate (bytes per
                                       second) to a device (default [])
      --device-write-iops list         Limit write rate (IO per second)
                                       to a device (default [])
      --disable-content-trust          Skip image verification (default true)
      --dns-option list                Set DNS options
      --dns-search list                Set custom DNS search domains
      --entrypoint string              Overwrite the default ENTRYPOINT
                                       of the image
      --env-file list                  Read in a file of environment variables
      --group-add list                 Add additional groups to join
      --health-cmd string              Command to run to check health
      --health-interval duration       Time between running the check
                                       (ms|s|m|h) (default 0s)
      --health-retries int             Consecutive failures needed to
                                       report unhealthy
      --health-start-period duration   Start period for the container to
                                       initialize before starting
                                       health-retries countdown
                                       (ms|s|m|h) (default 0s)
      --health-timeout duration        Maximum time to allow one check to
                                       run (ms|s|m|h) (default 0s)
      --help                           Print usage
      --init                           Run an init inside the container
                                       that forwards signals and reaps
                                       processes
  -i, --interactive                    Keep STDIN open even if not attached
      --io-maxbandwidth bytes          Maximum IO bandwidth limit for the
                                       system drive (Windows only)
      --io-maxiops uint                Maximum IOps limit for the system
                                       drive (Windows only)
      --ip string                      IPv4 address (e.g., 172.30.100.104)
      --ip6 string                     IPv6 address (e.g., 2001:db8::33)
      --ipc string                     IPC mode to use
      --isolation string               Container isolation technology
      --kernel-memory bytes            Kernel memory limit
      --label-file list                Read in a line delimited file of labels
      --link-local-ip list             Container IPv4/IPv6 link-local
                                       addresses

  -m, --memory bytes                   Memory limit
      --memory-reservation bytes       Memory soft limit
      --memory-swap bytes              Swap limit equal to memory plus
                                       swap: '-1' to enable unlimited swap
      --memory-swappiness int          Tune container memory swappiness
                                       (0 to 100) (default -1)
      --mount mount                    Attach a filesystem mount to the
                                       container

      --network-alias list             Add network-scoped alias for the
                                       container
      --no-healthcheck                 Disable any container-specified
                                       HEALTHCHECK
      --oom-kill-disable               Disable OOM Killer
      --oom-score-adj int              Tune host's OOM preferences (-1000
                                       to 1000)
      --pid string                     PID namespace to use
      --pids-limit int                 Tune container pids limit (set -1
                                       for unlimited)
      --platform string                Set platform if server is
                                       multi-platform capable

  -P, --publish-all                    Publish all exposed ports to
                                       random ports
      --read-only                      Mount the container's root
                                       filesystem as read only

      --rm                             Automatically remove the container
                                       when it exits
      --runtime string                 Runtime to use for this container
      --security-opt list              Security Options
      --shm-size bytes                 Size of /dev/shm
      --sig-proxy                      Proxy received signals to the
                                       process (default true)
      --stop-signal string             Signal to stop a container
                                       (default "SIGTERM")
      --stop-timeout int               Timeout (in seconds) to stop a
                                       container
      --storage-opt list               Storage driver options for the
                                       container
      --sysctl map                     Sysctl options (default map[])
      --tmpfs list                     Mount a tmpfs directory
      --ulimit ulimit                  Ulimit options (default [])

      --userns string                  User namespace to use
      --uts string                     UTS namespace to use
      --volume-driver string           Optional volume driver for the
                                       container

  -w, --workdir string                 Working directory inside the container
```
