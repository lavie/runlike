from runlike import Inspector
from subprocess import check_output, STDOUT, CalledProcessError


def test_command_empty():
    check_output("./fixtures.sh")
    ins = Inspector("runlike_fixture", True, False)
    ins.inspect()
    print ins.format_cli()


