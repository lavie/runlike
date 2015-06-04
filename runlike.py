#!/usr/bin/env python

import click

@click.command(help="Shows command line necessary to run copy of existing Docker container.")
@click.argument("container")
def cli():
    pass


def main():
    cli()

if __name__ == "__main__":
    main()
