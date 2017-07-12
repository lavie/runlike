import unittest
import os
from subprocess import check_output
from runlike import Inspector


class TestInspection(unittest.TestCase):

    @classmethod
    def setUpClass(cls):
        check_output("./fixtures.sh")
        ins = Inspector("runlike_fixture", True, True)
        ins.inspect()
        ins.pretty = True
        cls.output = ins.format_cli()

    def expect_substr(self, substr):
        self.assertIn(substr, TestInspection.output)

    def test_tcp_port(self):
        self.expect_substr("-p 300 \\")

    def test_tcp_port_with_host_port(self):
        self.expect_substr("-p 400:400 \\")

    def test_expose(self):
        self.expect_substr("--expose=1000 \\")

    def test_udp(self):
        self.expect_substr("-p 301/udp \\")

    def test_udp_with_host_port(self):
        self.expect_substr("-p 503:502/udp \\")

    def test_udp_with_host_port_and_ip(self):
        self.expect_substr("-p 127.0.0.1:601:600/udp \\")

    def test_host_volumes(self):
        cur_dir = os.path.dirname(os.path.realpath(__file__))
        self.expect_substr("--volume=\"%s:/workdir\"" % cur_dir)

    def test_no_host_volume(self):
        self.expect_substr('--volume="/random_volume"')

    def test_restart(self):
        self.expect_substr('--restart=always \\')
