from .inspector import Inspector
import sys

class Podman(Inspector):
    def __init__(self, container=None, no_name=None, pretty=None):
        super().__init__(container, no_name, pretty)
        self.executor = 'podman'

    def get_fact_name(self):
        return self.get_fact("Name")

    def parse_runtime(self):
        return