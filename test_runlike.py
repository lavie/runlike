import unittest
import os
from runlike import Inspector
from subprocess import check_output, STDOUT, CalledProcessError


class TestInspection(unittest.TestCase):

    def setUp(self):
        check_output("./fixtures.sh")
        ins = Inspector("runlike_fixture", True, False)
        ins.inspect()
        ins.pretty = True
        self.output = ins.format_cli()

    def expect_substr(self, substr):
        self.assertIn(substr, self.output)

    def test_ports(self):
        self.expect_substr("-p 0.0.0.0:400:400/tcp")

    def test_volumes(self):
        cur_dir = os.path.dirname(os.path.realpath(__file__))
        self.expect_substr("--volume=\"%s:/workdir\"" % cur_dir)


