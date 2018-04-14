import unittest
import os
import pipes
from subprocess import check_output
from runlike.inspector import Inspector


class TestInspection(unittest.TestCase):

    @classmethod
    def setUpClass(cls):
        check_output("./fixtures.sh")
        cls.outputs = {}
        for i in range(5):

            ins = Inspector("runlike_fixture%d" % (i + 1), True, True)
            ins.inspect()
            cls.outputs[i + 1] = ins.format_cli()

    def expect_substr(self, substr, fixture_index=1):
        hay = TestInspection.outputs[fixture_index]
        if substr not in hay:
            print("Expecting to find:{substr}\nInside:\n{hay}\n".
                  format(substr=substr, hay=hay))
            self.fail()

    def dont_expect_substr(self, substr, fixture_index=1):
        self.assertNotIn(substr, TestInspection.outputs[fixture_index])

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
        self.expect_substr("--volume=%s:/workdir" % pipes.quote(cur_dir))

    def test_no_host_volume(self):
        self.expect_substr('--volume=/random_volume')

    def test_tty(self):
        self.expect_substr('-t \\')
        self.dont_expect_substr('-t \\', 2)

    def test_restart_always(self):
        self.expect_substr('--restart=always \\')

    def test_restart_on_failure(self):
        self.expect_substr('--restart=on-failure \\', 2)

    def test_restart_with_max(self):
        self.expect_substr('--restart=on-failure:3 \\', 3)

    def test_hostname(self):
        self.expect_substr('--hostname=Essos \\')

    def test_hostname_not_present(self):
        self.dont_expect_substr('--hostname \\', 2)

    def test_network_mode(self):
        self.dont_expect_substr('--network', 1)
        self.expect_substr('--network=host', 2)
        self.expect_substr('--network=runlike_fixture_bridge', 3)

    def test_privileged_mode(self):
        self.expect_substr('--privileged \\')

    def test_privileged_not_present(self):
        self.dont_expect_substr('--privileged \\', 2)

    def test_multi_labels(self):
        self.expect_substr('--label com.example.environment="test" \\', 1)
        self.expect_substr('--label com.example.group="one" \\', 1)

    def test_one_label(self):
        self.expect_substr('--label com.example.version="1" \\', 2)

    def test_labels_not_present(self):
        self.dont_expect_substr('--label', 3)

    def test_links(self):
        self.expect_substr('--link runlike_fixture4:alias_of4 \\', 5)
        self.expect_substr('--link runlike_fixture1 \\', 5)

    def test_command(self):
        self.dont_expect_substr('/bin/bash', 1)
        self.expect_substr('/bin/bash sleep.sh', 2)
        self.expect_substr("bash -c 'bash sleep.sh'", 3)
        self.expect_substr(r"bash -c 'bash \'sleep.sh\'", 4)

    def test_user(self):
        self.expect_substr('--user=daemon')
        self.dont_expect_substr('--user', 2)

    def test_mac_address(self):
        self.expect_substr('--mac-address=6a:00:01:ad:d9:e0', 4)
        self.dont_expect_substr('--mac-address', 2)

    def test_env(self):
        val = '''FOO=thing="quoted value with 'spaces' and 'single quotes'"'''
        self.expect_substr("""--env=%s""" % pipes.quote(val))

    def test_cap_add(self):
        self.expect_substr("--cap-add=CHOWN")
