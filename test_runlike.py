import unittest
import os
from subprocess import check_output
from runlike.inspector import Inspector


class TestInspection(unittest.TestCase):

    @classmethod
    def setUpClass(cls):
        check_output("./fixtures.sh")
        cls.outputs = {}
        for i in range(3):

            ins = Inspector("runlike_fixture%d" % (i + 1), True, True)
            ins.inspect()
            cls.outputs[i + 1] = ins.format_cli()

    def expect_substr(self, substr, fixture_index=1):
        self.assertIn(substr, TestInspection.outputs[fixture_index])

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

    def test_restart_always(self):
        self.expect_substr('--restart=always \\')

    def test_restart_on_failure(self):
        self.expect_substr('--restart=on-failure \\', 2)

    def test_restart_with_max(self):
        self.expect_substr('--restart=on-failure:3 \\', 3)
