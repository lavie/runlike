import sys
import re
from subprocess import (
    check_output,
    STDOUT,
    CalledProcessError
)
from json import loads, dumps
from pipes import quote


def die(message):
    sys.stderr.write(message + "\n")
    sys.exit(1)


class Inspector(object):

    def __init__(self, container=None, no_name=None, pretty=None):
        self.container = container
        self.no_name = no_name
        self.output = ""
        self.pretty = pretty
        self.facts = None
        self.options = []

    def inspect(self):
        try:
            output = check_output(
                ["docker", "container", "inspect", self.container],
                stderr=STDOUT)
            self.facts = loads(output.decode())
        except CalledProcessError as e:
            if b"No such container" in e.output:
                die("No such container %s" % self.container)
            else:
                die(str(e))

    def set_facts(self, raw_json):
        self.facts = loads(raw_json)

    def get_fact(self, path):
        parts = path.split(".")
        value = self.facts[0]
        for p in parts:
            if p not in value:
                return None
            value = value[p]
        return value

    def multi_option(self, path, option):
        values = self.get_fact(path)
        if values:
            for val in values:
                self.options.append('--%s=%s' % (option, quote(val)))

    def parse_hostname(self):
        hostname = self.get_fact("Config.Hostname")
        self.options.append("--hostname=%s" % hostname)

    def parse_user(self):
        user = self.get_fact("Config.User")
        if user != "":
            self.options.append("--user=%s" % user)

    def parse_macaddress(self):
        try:
            mac_address = self.get_fact("Config.MacAddress") or self.get_fact("NetworkSettings.MacAddress") or {}
            if mac_address:
                self.options.append("--mac-address=%s" % mac_address)
        except Exception:
            pass

    def parse_ports(self):
        ports = self.get_fact("NetworkSettings.Ports") or {}
        ports.update(self.get_fact("HostConfig.PortBindings") or {})

        if ports:
            for container_port_and_protocol, options in ports.items():
                container_port, protocol = container_port_and_protocol.split('/')
                protocol_part = '' if protocol == 'tcp' else '/udp'
                option_part = '-p '
                host_port_part = ''
                hostname_part = ''

                if options is None:
                    # --expose
                    option_part = '--expose='
                else:
                    # -p
                    host_ip = options[0]['HostIp']
                    host_port = options[0]['HostPort']

                    if host_port != '0' and host_port != '':
                        host_port_part = f"{host_port}:"

                    if host_ip not in ['0.0.0.0', '']:
                        hostname_part = f"{host_ip}:"

                self.options.append(f"{option_part}{hostname_part}{host_port_part}{container_port}{protocol_part}")

    def parse_links(self):
        links = self.get_fact("HostConfig.Links")
        link_options = set()
        if links is not None:
            for link in links:
                src, dst = link.split(":")
                dst = dst.split("/")[-1]
                src = src.split("/")[-1]
                if src != dst:
                    link_options.add('--link %s:%s' % (src, dst))
                else:
                    link_options.add('--link %s' % (src))

        self.options += list(link_options)

    def parse_pid(self):
        mode = self.get_fact("HostConfig.PidMode")
        if mode != "":
            self.options.append("--pid %s" % mode)

    def parse_restart(self):
        restart = self.get_fact("HostConfig.RestartPolicy.Name")
        if not restart:
            return
        elif restart == 'on-failure':
            max_retries = self.get_fact(
                "HostConfig.RestartPolicy.MaximumRetryCount")
            if max_retries > 0:
                restart += ":%d" % max_retries
        self.options.append("--restart=%s" % restart)

    def parse_devices(self):
        devices = self.get_fact("HostConfig.Devices")
        if not devices:
            return
        device_options = set()
        for device_spec in devices:
            host = device_spec['PathOnHost']
            container = device_spec['PathInContainer']
            perms = device_spec['CgroupPermissions']
            spec = '%s:%s' % (host, container)
            if perms != 'rwm':
                spec += ":%s" % perms
            device_options.add('--device %s' % (spec,))

        self.options += list(device_options)

    def parse_labels(self):
        labels = self.get_fact("Config.Labels") or {}
        label_options = set()
        if labels is not None:
            for key, value in labels.items():
                label_options.add("--label='%s=%s'" % (key, value))
        self.options += list(label_options)

    def parse_log(self):
        log_type = self.get_fact("HostConfig.LogConfig.Type")
        log_opts = self.get_fact("HostConfig.LogConfig.Config") or {}
        log_options = set()
        if log_type != 'json-file':
            log_options.add('--log-driver=%s' % log_type)
        if log_opts:
            for key, value in log_opts.items():
                log_options.add('--log-opt %s=%s' % (key, value))
        self.options += list(log_options)

    def parse_extra_hosts(self):
        hosts = self.get_fact("HostConfig.ExtraHosts") or []
        self.options += ['--add-host %s' % host for host in hosts]

    def parse_workdir(self):
        workdir = self.get_fact("Config.WorkingDir")
        if workdir:
            self.options.append("--workdir=%s" % workdir)

    def parse_runtime(self):
        runtime = self.get_fact("HostConfig.Runtime")
        if runtime:
            self.options.append("--runtime=%s" % runtime)

    def format_cli(self):
        self.output = "docker run "

        image = self.get_fact("Config.Image")
        self.options = []

        name = self.get_fact("Name").split("/")[1]
        if not self.no_name:
            self.options.append("--name=%s" % name)
        self.parse_hostname()
        self.parse_user()
        self.parse_macaddress()
        self.parse_pid()

        self.multi_option("Config.Env", "env")
        self.multi_option("HostConfig.Binds", "volume")
        self.multi_option("Config.Volumes", "volume")
        self.multi_option("HostConfig.VolumesFrom", "volumes-from")
        self.multi_option("HostConfig.CapAdd", "cap-add")
        self.multi_option("HostConfig.CapDrop", "cap-drop")
        self.multi_option("HostConfig.Dns", "dns")
        network_mode = self.get_fact("HostConfig.NetworkMode")
        if network_mode != "default":
            self.options.append("--network=" + network_mode)
        privileged = self.get_fact('HostConfig.Privileged')
        if privileged:
            self.options.append("--privileged")

        self.parse_workdir()
        self.parse_ports()
        self.parse_links()
        self.parse_restart()
        self.parse_devices()
        self.parse_labels()
        self.parse_log()
        self.parse_extra_hosts()
        self.parse_runtime()

        stdout_attached = self.get_fact("Config.AttachStdout")
        if not stdout_attached:
            self.options.append("--detach=true")

        if self.get_fact("Config.Tty"):
            self.options.append('-t')

        parameters = ["run"]
        if self.options:
            parameters += self.options
        parameters.append(image)

        cmd_parts = self.get_fact("Config.Cmd")
        if cmd_parts:
            # NOTE: pipes.quote() performs syntactically correct
            # quoting and replace operation below is needed just for
            # aesthetic reasons and visual similarity with old output.
            quoted = [
                quote(p).replace("'\"'\"'", r"\'")
                for p in cmd_parts
            ]
            command = " ".join(quoted)
            parameters.append(command)

        joiner = " "
        if self.pretty:
            joiner += "\\\n\t"
        parameters = joiner.join(parameters)

        return "docker %s" % parameters
