#!/usr/bin/env python

import click
from subprocess import check_output, STDOUT, CalledProcessError
from json import loads
import sys

def die(message):
    sys.stderr.write(message + "\n")
    sys.exit(1)


class Inspector(object):
    def __init__(self, container, no_name):
        self.container = container
        self.no_name = no_name

    def inspect(self):
        try:
            output = check_output("docker inspect %s" % self.container, stderr=STDOUT, shell=True)
            self.facts = loads(output)
        except CalledProcessError, e:
            if "No such image or container" in e.output:
                die("No such container %s" % self.container)
            else:
                die(str(e))

    def format_cli(self):

        return "docker run ..."



@click.command(help="Shows command line necessary to run copy of existing Docker container.")
@click.argument("container")
@click.option("--no-name", is_flag=True, help="Do not include container name in output")
def cli(container, no_name):
    ins = Inspector(container, no_name)
    ins.inspect()
    print ins.format_cli()
    


def main():
    cli()

if __name__ == "__main__":
    main()
