#!/usr/bin/env python

import click

try:
    from .inspector import Inspector
except ValueError:
    from inspector import Inspector


@click.command(
    help="Shows command line necessary to run copy of existing Docker container.")
@click.argument("container", required=False )
@click.option(
    "--no-name",
    is_flag=True,
    help="Do not include container name in output")
@click.option("-p", "--pretty", is_flag=True)
@click.option("-s", "--stdin", is_flag=True)
def cli(container, no_name, pretty, stdin):

    # TODO: -i, -t, -d as added options that override the inspection
    if container:
        ins = Inspector(container, no_name, pretty)
        ins.inspect()
        print(ins.format_cli())
    elif stdin:
        ins = Inspector()
        ins.pretty = pretty
        raw_json = click.get_text_stream('stdin').read()
        ins.set_facts(raw_json)
        print(ins.format_cli())
    else: 
        raise click.UsageError("usage error")

def main():
    cli()

if __name__ == "__main__":
    main()
