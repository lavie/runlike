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

`runlike` is packaged as a Docker image called [assaflavie/runlike](https://hub.docker.com/r/assaflavie/runlike/). 

```
docker run --rm -v /var/run/docker.sock:/var/run/docker.sock \
    assaflavie/runlike YOUR-CONTAINER
```

Or you can run it with alias, for example, save it in `~/.profile` or `~/.bashrc`

```
alias runlike="docker run --rm -v /var/run/docker.sock:/var/run/docker.sock assaflavie/runlike"
```

Then you can run as local command directly

```
runlike YOUR-CONTAINER
```

# Install

    $ pip install runlike

# Status

This is very much a work in progress. Many `docker run` options aren't yet supported, but the most commonly used ones are. Feel free to send pull requests if you add any or if you happen to fix any (of the many) bugs this package undoubtedly has.

Probably **shouldn't use this in production** yet. If you do, double check that it's actually running what you want it to run.

[![Build Status](https://travis-ci.org/lavie/runlike.svg?branch=master)](https://travis-ci.org/lavie/runlike)
