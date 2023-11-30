import logging

from mitmproxy import ctx
from mitmproxy.http import Response

class StatusCode:
    def __init__(self):
        self.return_status_code = 0 # disabled

    def load(self, loader):
        loader.add_option(
            name="statuscode",
            typespec=int,
            default=0,
            help="Change the response status code",
        )

    def configure(self, updates):
        if "statuscode" not in updates:
            self.return_status_code = 0
            return
        if ctx.options.statuscode is None or ctx.options.statuscode == 0:
            self.return_status_code = 0
            return
        self.return_status_code = ctx.options.statuscode
        logging.info(f"statuscode will return HTTP {self.return_status_code}")

    def response(self, flow):
        if not flow.request.pretty_host.startswith("hs"):
            return # ignore responses sent by the controller
        if self.return_status_code == 0:
            return # ignore responses if we aren't told a code
        flow.response = Response.make(self.return_status_code)
